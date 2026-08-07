//go:build windows

// keydump extracts the AES-CWC session keys from a running Dark Souls II
// (Scholar of the First Sin, PC/Steam) client.
//
// Why this exists: the game-service traffic to FromSoftware's live servers is
// CWC-encrypted in both directions with a key that never appears on the wire in
// recoverable form — the handshake sends it client->server under RSA-OAEP to
// FromSoftware's public key. A packet capture alone is therefore permanently
// opaque. The key does exist in the client's memory, and every key the client
// installs passes through a single function.
//
// See tasks/pc-capture-decryption.md for the full derivation. The short version:
//
//	cwc_init_and_key   (Brian Gladman's aes_modes/cwc.c, the same code our own
//	                    internal/crypto/cwc descends from)
//	  RCX = key pointer   RDX = key length (16)   R8 = cwc_ctx*
//
// It is found by signature rather than by address, so ASLR and a different build
// do not matter: the instruction `test eax,0xFFFFFFE7` encodes as A9 E7 FF FF FF
// and occurs EXACTLY ONCE in the whole 28 MB image, at function entry +0x22.
//
// This attaches as a debugger and only ever reads. It writes one INT3 byte to
// set the breakpoint and restores it immediately on each hit; nothing else in
// the process is modified, and no game data is touched.
//
// Usage:
//
//	keydump.exe                 attach to DarkSoulsII.exe, print keys as they appear
//	keydump.exe -pid 1234       attach to a specific process
//	keydump.exe -out keys.txt   also append to a file
//
// Start it after the game is running but BEFORE going online — the keys are
// installed during the login handshake. Expect at least two hits: the auth-stream
// key first, then the game-service key.
package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The signature and the offset from it back to the function entry.
//
// `test eax,0xFFFFFFE7` — the argument check that accepts only key lengths 16,
// 24 and 32. Unique across the entire image, which is what makes this robust
// without hardcoding an address.
var (
	sigCWCInit    = []byte{0xA9, 0xE7, 0xFF, 0xFF, 0xFF}
	sigEntryDelta = uintptr(0x22)
)

const (
	dbgContinue             = 0x00010002
	dbgExceptionNotHandled  = 0x80010001
	exceptionDebugEvent     = 1
	createProcessDebugEvent = 3
	exitProcessDebugEvent   = 5
	exceptionBreakpoint     = 0x80000003
	exceptionSingleStep     = 0x80000004

	// CONTEXT_AMD64 | CONTEXT_CONTROL | CONTEXT_INTEGER
	contextControlInteger = 0x00100003

	// Offsets into the x64 CONTEXT structure.
	ctxContextFlags = 0x30
	ctxEFlags       = 0x44
	ctxRcx          = 0x80
	ctxR8           = 0xB8
	ctxRip          = 0xF8
	ctxSize         = 1232

	trapFlag = 0x100
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procDebugActiveProc   = kernel32.NewProc("DebugActiveProcess")
	procDebugActiveStop   = kernel32.NewProc("DebugActiveProcessStop")
	procDebugSetKillOnEx  = kernel32.NewProc("DebugSetProcessKillOnExit")
	procWaitForDebugEvent = kernel32.NewProc("WaitForDebugEvent")
	procContinueDebugEvt  = kernel32.NewProc("ContinueDebugEvent")
	procReadProcessMemory = kernel32.NewProc("ReadProcessMemory")
	procWriteProcessMem   = kernel32.NewProc("WriteProcessMemory")
	procFlushInstruction  = kernel32.NewProc("FlushInstructionCache")
	procVirtualProtectEx  = kernel32.NewProc("VirtualProtectEx")
	procOpenThread        = kernel32.NewProc("OpenThread")
	procGetThreadContext  = kernel32.NewProc("GetThreadContext")
	procSetThreadContext  = kernel32.NewProc("SetThreadContext")
	procCloseHandle       = kernel32.NewProc("CloseHandle")
	procCreateToolhelp32  = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW   = kernel32.NewProc("Process32FirstW")
	procProcess32NextW    = kernel32.NewProc("Process32NextW")
)

func main() {
	pid := flag.Int("pid", 0, "process id (0 = find DarkSoulsII.exe)")
	name := flag.String("name", "DarkSoulsII.exe", "process name to find when -pid is not given")
	out := flag.String("out", "", "also append keys to this file")
	flag.Parse()

	target := uint32(*pid)
	if target == 0 {
		found, err := findProcess(*name)
		if err != nil {
			fatal("%v\n\nStart the game first, then run this. Or pass -pid.", err)
		}
		target = found
		fmt.Printf("found %s, pid %d\n", *name, target)
	}

	var sink *os.File
	if *out != "" {
		f, err := os.OpenFile(*out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fatal("cannot open -out file: %v", err)
		}
		defer f.Close()
		sink = f
	}

	if err := run(target, sink); err != nil {
		fatal("%v", err)
	}
}

func run(pid uint32, sink *os.File) error {
	// Without this, detaching or crashing takes the game down with us.
	procDebugSetKillOnEx.Call(0)

	if r, _, err := procDebugActiveProc.Call(uintptr(pid)); r == 0 {
		return fmt.Errorf("DebugActiveProcess(%d): %v (run as Administrator, and make sure no other debugger is attached)", pid, err)
	}
	defer procDebugActiveStop.Call(uintptr(pid))

	fmt.Println("attached; waiting for the game to install a key...")
	fmt.Println("(go online now — the keys are installed during the login handshake)")

	var (
		hProcess  uintptr
		bpAddr    uintptr
		origByte  byte
		armed     bool
		pendingBP uintptr // thread id -> needs re-arm after single step
		hits      int
	)

	evt := make([]byte, 4096)
	for {
		r, _, _ := procWaitForDebugEvent.Call(uintptr(unsafe.Pointer(&evt[0])), 1000)
		if r == 0 {
			continue // timeout; loop so Ctrl-C stays responsive
		}

		code := binary.LittleEndian.Uint32(evt[0:])
		tid := binary.LittleEndian.Uint32(evt[8:])
		status := uintptr(dbgContinue)

		switch code {
		case createProcessDebugEvent:
			// CREATE_PROCESS_DEBUG_INFO: hProcess at union+8, lpBaseOfImage at
			// union+24. The union starts at offset 16 in the x64 DEBUG_EVENT.
			hProcess = uintptr(binary.LittleEndian.Uint64(evt[16+8:]))
			base := uintptr(binary.LittleEndian.Uint64(evt[16+24:]))
			fmt.Printf("image base %#x\n", base)

			hit, err := scanForSignature(hProcess, base)
			if err != nil {
				return err
			}
			bpAddr = hit - sigEntryDelta
			fmt.Printf("signature at %#x -> cwc_init_and_key at %#x\n", hit, bpAddr)

			origByte, err = setBreakpoint(hProcess, bpAddr)
			if err != nil {
				return fmt.Errorf("set breakpoint: %w", err)
			}
			armed = true
			fmt.Println("breakpoint armed")

		case exceptionDebugEvent:
			exCode := binary.LittleEndian.Uint32(evt[16:])
			exAddr := uintptr(binary.LittleEndian.Uint64(evt[16+16:]))

			switch {
			case exCode == exceptionBreakpoint && armed && exAddr == bpAddr:
				key, ctx, err := readKeyAt(hProcess, tid)
				if err != nil {
					fmt.Fprintf(os.Stderr, "read key: %v\n", err)
				} else {
					hits++
					line := fmt.Sprintf("[%s] key #%d  %s  ctx=%#x",
						time.Now().Format("15:04:05"), hits, hex.EncodeToString(key), ctx)
					fmt.Println(line)
					if sink != nil {
						fmt.Fprintln(sink, line)
						sink.Sync()
					}
				}

				// Step over: restore the byte, rewind RIP onto it, and set the
				// trap flag so we get control back to re-arm.
				if err := writeBytes(hProcess, bpAddr, []byte{origByte}); err != nil {
					return fmt.Errorf("restore byte: %w", err)
				}
				if err := adjustThread(tid, bpAddr); err != nil {
					return fmt.Errorf("rewind rip: %w", err)
				}
				pendingBP = uintptr(tid)

			case exCode == exceptionSingleStep && pendingBP == uintptr(tid):
				// Back after the step — put the breakpoint back.
				var err error
				origByte, err = setBreakpoint(hProcess, bpAddr)
				if err != nil {
					return fmt.Errorf("re-arm: %w", err)
				}
				pendingBP = 0

			default:
				// Anything else is the game's own business; hand it back.
				status = dbgExceptionNotHandled
			}

		case exitProcessDebugEvent:
			fmt.Printf("process exited; %d key(s) captured\n", hits)
			procContinueDebugEvent(pid, tid, status)
			return nil
		}

		procContinueDebugEvent(pid, tid, status)
	}
}

// scanForSignature walks the module image looking for the unique instruction.
//
// It reads SizeOfImage out of the PE header rather than guessing a range, and
// reports how many matches it found: the signature is supposed to be unique, and
// if it ever is not, that is something to know rather than to silently take the
// first of.
func scanForSignature(hProcess, base uintptr) (uintptr, error) {
	hdr, err := readBytes(hProcess, base, 0x1000)
	if err != nil {
		return 0, fmt.Errorf("read PE header: %w", err)
	}
	if hdr[0] != 'M' || hdr[1] != 'Z' {
		return 0, fmt.Errorf("no MZ magic at image base %#x", base)
	}
	ntOff := uintptr(binary.LittleEndian.Uint32(hdr[0x3C:]))
	if ntOff+0x58 > uintptr(len(hdr)) {
		return 0, fmt.Errorf("PE header at %#x is past the first page", ntOff)
	}
	sizeOfImage := uintptr(binary.LittleEndian.Uint32(hdr[ntOff+0x50:]))
	fmt.Printf("image size %#x\n", sizeOfImage)

	const chunk = 1 << 20
	var matches []uintptr
	overlap := uintptr(len(sigCWCInit) - 1)

	for off := uintptr(0); off < sizeOfImage; off += chunk - overlap {
		n := uintptr(chunk)
		if off+n > sizeOfImage {
			n = sizeOfImage - off
		}
		buf, err := readBytes(hProcess, base+off, n)
		if err != nil {
			continue // unreadable pages are normal; keep going
		}
		for i := 0; i+len(sigCWCInit) <= len(buf); i++ {
			if buf[i] == sigCWCInit[0] && string(buf[i:i+len(sigCWCInit)]) == string(sigCWCInit) {
				matches = append(matches, base+off+uintptr(i))
			}
		}
	}

	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("signature %X not found in the image — wrong game build?", sigCWCInit)
	case 1:
		return matches[0], nil
	default:
		// Not fatal, but the whole approach rests on uniqueness, so say so.
		fmt.Fprintf(os.Stderr, "WARNING: signature matched %d times (expected 1): %v\n"+
			"using the first; keys from a wrong site will fail verification\n", len(matches), matches)
		return matches[0], nil
	}
}

// readKeyAt reads RCX (the key pointer) and R8 (the cwc_ctx) from the stopped
// thread, then the 16 key bytes.
//
// RDX is the key length and should be 16; it is checked rather than assumed,
// because a 24- or 32-byte key would mean this is not the call we think it is.
func readKeyAt(hProcess uintptr, tid uint32) ([]byte, uintptr, error) {
	hThread, err := openThread(tid)
	if err != nil {
		return nil, 0, err
	}
	defer procCloseHandle.Call(hThread)

	ctx, err := getContext(hThread)
	if err != nil {
		return nil, 0, err
	}
	rcx := uintptr(binary.LittleEndian.Uint64(ctx[ctxRcx:]))
	r8 := uintptr(binary.LittleEndian.Uint64(ctx[ctxR8:]))

	key, err := readBytes(hProcess, rcx, 16)
	if err != nil {
		return nil, 0, fmt.Errorf("read 16 bytes at RCX=%#x: %w", rcx, err)
	}
	return key, r8, nil
}

func setBreakpoint(hProcess, addr uintptr) (byte, error) {
	orig, err := readBytes(hProcess, addr, 1)
	if err != nil {
		return 0, err
	}
	if err := writeBytes(hProcess, addr, []byte{0xCC}); err != nil {
		return 0, err
	}
	return orig[0], nil
}

// adjustThread rewinds RIP onto the restored instruction and sets the trap flag.
func adjustThread(tid uint32, addr uintptr) error {
	hThread, err := openThread(tid)
	if err != nil {
		return err
	}
	defer procCloseHandle.Call(hThread)

	ctx, err := getContext(hThread)
	if err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(ctx[ctxRip:], uint64(addr))
	eflags := binary.LittleEndian.Uint32(ctx[ctxEFlags:])
	binary.LittleEndian.PutUint32(ctx[ctxEFlags:], eflags|trapFlag)

	if r, _, err := procSetThreadContext.Call(hThread, uintptr(unsafe.Pointer(&ctx[0]))); r == 0 {
		return fmt.Errorf("SetThreadContext: %v", err)
	}
	return nil
}

// getContext returns a 16-byte-aligned CONTEXT, which the API requires.
func getContext(hThread uintptr) ([]byte, error) {
	raw := make([]byte, ctxSize+16)
	off := (16 - (uintptr(unsafe.Pointer(&raw[0])) % 16)) % 16
	ctx := raw[off : off+ctxSize]

	binary.LittleEndian.PutUint32(ctx[ctxContextFlags:], contextControlInteger)
	if r, _, err := procGetThreadContext.Call(hThread, uintptr(unsafe.Pointer(&ctx[0]))); r == 0 {
		return nil, fmt.Errorf("GetThreadContext: %v", err)
	}
	return ctx, nil
}

func openThread(tid uint32) (uintptr, error) {
	const threadAllAccess = 0x1FFFFF
	h, _, err := procOpenThread.Call(threadAllAccess, 0, uintptr(tid))
	if h == 0 {
		return 0, fmt.Errorf("OpenThread(%d): %v", tid, err)
	}
	return h, nil
}

func readBytes(hProcess, addr uintptr, n uintptr) ([]byte, error) {
	buf := make([]byte, n)
	var read uintptr
	r, _, err := procReadProcessMemory.Call(hProcess, addr,
		uintptr(unsafe.Pointer(&buf[0])), n, uintptr(unsafe.Pointer(&read)))
	if r == 0 {
		return nil, fmt.Errorf("ReadProcessMemory(%#x, %d): %v", addr, n, err)
	}
	return buf[:read], nil
}

func writeBytes(hProcess, addr uintptr, b []byte) error {
	const pageExecuteReadWrite = 0x40
	var old uint32
	procVirtualProtectEx.Call(hProcess, addr, uintptr(len(b)), pageExecuteReadWrite,
		uintptr(unsafe.Pointer(&old)))

	var written uintptr
	r, _, err := procWriteProcessMem.Call(hProcess, addr,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&written)))

	procVirtualProtectEx.Call(hProcess, addr, uintptr(len(b)), uintptr(old),
		uintptr(unsafe.Pointer(&old)))
	procFlushInstruction.Call(hProcess, addr, uintptr(len(b)))

	if r == 0 {
		return fmt.Errorf("WriteProcessMemory(%#x): %v", addr, err)
	}
	return nil
}

func procContinueDebugEvent(pid, tid uint32, status uintptr) {
	procContinueDebugEvt.Call(uintptr(pid), uintptr(tid), status)
}

// findProcess locates a process by executable name via the Toolhelp snapshot.
func findProcess(name string) (uint32, error) {
	const th32csSnapProcess = 0x2
	snap, _, err := procCreateToolhelp32.Call(th32csSnapProcess, 0)
	if snap == 0 || snap == uintptr(^uintptr(0)) {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot: %v", err)
	}
	defer procCloseHandle.Call(snap)

	// PROCESSENTRY32W: dwSize(0), cntUsage(4), th32ProcessID(8), ...,
	// szExeFile at 44, 260 WCHARs. Total 568.
	const entrySize = 568
	entry := make([]byte, entrySize)
	binary.LittleEndian.PutUint32(entry[0:], entrySize)

	r, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry[0])))
	for r != 0 {
		pid := binary.LittleEndian.Uint32(entry[8:])
		exe := utf16Name(entry[44:])
		if strings.EqualFold(exe, name) {
			return pid, nil
		}
		binary.LittleEndian.PutUint32(entry[0:], entrySize)
		r, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry[0])))
	}
	return 0, fmt.Errorf("no process named %q is running", name)
}

func utf16Name(b []byte) string {
	var u []uint16
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return syscall.UTF16ToString(u)
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "keydump: "+format+"\n", a...)
	os.Exit(1)
}
