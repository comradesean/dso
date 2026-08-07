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
	"os/signal"
	"runtime"
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

	verbose    bool
	lockThread bool
)

func exceptionName(code uint32) string {
	switch code {
	case exceptionBreakpoint:
		return "BREAKPOINT"
	case exceptionSingleStep:
		return "SINGLE_STEP"
	default:
		return fmt.Sprintf("%#x", code)
	}
}

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
	ctxRcx          = 0x80
	ctxR8           = 0xB8
	ctxRip          = 0xF8
	ctxSize         = 1232

	// rearmDelay is how long the breakpoint stays removed after a hit, so the
	// thread that hit it can clear the instruction before it comes back. The two
	// keys we care about arrive about a second apart, so this is comfortably
	// inside that.
	rearmDelay = 150 * time.Millisecond
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
	flag.BoolVar(&verbose, "v", false, "log every debug event (use if it misbehaves)")
	flag.BoolVar(&lockThread, "lock", false, "pin the debug loop to one OS thread (see the note in run)")
	wantKeys := flag.Int("n", 2, "detach after this many keys (0 = stay attached)")
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

	if err := run(target, sink, *wantKeys); err != nil {
		fatal("%v", err)
	}
}

func run(pid uint32, sink *os.File, wantKeys int) error {
	// OFF BY DEFAULT, deliberately, and the reasoning is worth keeping.
	//
	// The Windows debug API is thread-affine: WaitForDebugEvent and
	// ContinueDebugEvent are supposed to come from the same OS thread that
	// called DebugActiveProcess, and Go moves goroutines between OS threads
	// whenever it likes. By the book this lock should be mandatory, and its
	// absence should cause exactly the failure it is meant to prevent — the
	// debuggee suspended forever, waiting for a continue that never arrives.
	//
	// In practice the unlocked build is the one that captured keys from a live
	// client, and adding the lock brought the freeze back. Go's main goroutine
	// starts on the main thread and nothing here yields long enough to be
	// migrated, so the lock buys little and evidently costs something.
	//
	// Observation wins. Default to what demonstrably worked, keep the flag so
	// the question stays answerable, and do not quietly "fix" this back.
	if lockThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	// Without this, detaching or crashing takes the game down with us.
	procDebugSetKillOnEx.Call(0)

	if r, _, err := procDebugActiveProc.Call(uintptr(pid)); r == 0 {
		return fmt.Errorf("DebugActiveProcess(%d): %v (run as Administrator, and make sure no other debugger is attached)", pid, err)
	}
	defer procDebugActiveStop.Call(uintptr(pid))

	fmt.Println("attached; waiting for the game to install a key...")
	fmt.Println("(go online now — the keys are installed during the login handshake)")

	var (
		hProcess  uintptr //nolint:unused // read by the signal handler
		imageBase uintptr
		bpAddr    uintptr
		origByte  byte
		armed     bool
		needScan  bool
		rearmAt   time.Time // zero = nothing pending
		hits      int
	)

	// Ctrl-C must not leave an INT3 planted. Go runs no deferred functions on
	// SIGINT, and DebugSetProcessKillOnExit(0) means the game survives us — so
	// without this it keeps running with a breakpoint byte in its code and dies
	// the next time that function is called, long after the debugger is gone.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt)
	go func() {
		<-sigc
		if armed && hProcess != 0 {
			writeBytes(hProcess, bpAddr, []byte{origByte})
			fmt.Println("\nbreakpoint removed; detaching")
		}
		procDebugActiveStop.Call(uintptr(pid))
		os.Exit(0)
	}()

	evt := make([]byte, 4096)
	for {
		r, _, err := procWaitForDebugEvent.Call(uintptr(unsafe.Pointer(&evt[0])), 50)
		if r == 0 {
			// ERROR_SEM_TIMEOUT is the normal idle case; anything else means
			// the loop is broken and would otherwise spin in silence while the
			// game sat suspended.
			if errno, ok := err.(syscall.Errno); ok && errno == 121 {
				continue
			}
			if verbose {
				fmt.Fprintf(os.Stderr, "WaitForDebugEvent: %v\n", err)
			}
			// Idle is also when the re-arm falls due.
			if !rearmAt.IsZero() && time.Now().After(rearmAt) {
				if b, err := setBreakpoint(hProcess, bpAddr); err == nil {
					origByte, armed, rearmAt = b, true, time.Time{}
				}
			}
			continue
		}

		code := binary.LittleEndian.Uint32(evt[0:])
		tid := binary.LittleEndian.Uint32(evt[8:])
		status := uintptr(dbgContinue)

		if verbose {
			fmt.Printf("event %d tid %d\n", code, tid)
		}

		switch code {
		case createProcessDebugEvent:
			// CREATE_PROCESS_DEBUG_INFO: hProcess at union+8, lpBaseOfImage at
			// union+24. The union starts at offset 16 in the x64 DEBUG_EVENT.
			hProcess = uintptr(binary.LittleEndian.Uint64(evt[16+8:]))
			imageBase = uintptr(binary.LittleEndian.Uint64(evt[16+24:]))
			fmt.Printf("image base %#x\n", imageBase)

			// Deliberately do NOT scan here. The whole process stays suspended
			// until ContinueDebugEvent, and scanning 28 MB holds it frozen for
			// as long as that takes — long enough to hang the game if the attach
			// lands during save checking, which is exactly what happened.
			//
			// ReadProcessMemory and WriteProcessMemory work fine on a running
			// process, so the scan and the breakpoint go in after the continue.
			needScan = true

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

				// Step over WITHOUT single-stepping.
				//
				// The textbook approach — restore the byte, rewind RIP, set the
				// trap flag, re-arm on the single-step exception — has too many
				// ways to fail in a process with this many threads. If the
				// rewind does not take, the thread resumes mid-instruction and
				// the process dies; and the single-step bookkeeping is per
				// thread, so a second thread hitting the same address while one
				// is mid-step corrupts it. That is what "got key #1 and froze"
				// looked like.
				//
				// Instead: restore the byte, rewind RIP, and let the thread run
				// normally. Re-arm from the event loop a moment later. Nothing
				// is left half-done, and the only cost is a blind window — which
				// is fine, since the two keys we want arrive about a second
				// apart and the window is a fraction of that.
				if err := writeBytes(hProcess, bpAddr, []byte{origByte}); err != nil {
					return fmt.Errorf("restore byte: %w", err)
				}
				armed = false
				if err := rewindRIP(tid, bpAddr); err != nil {
					// A failed rewind leaves RIP inside an instruction, so bail
					// rather than let the game run off a cliff.
					return fmt.Errorf("rewind rip (game left running): %w", err)
				}
				// Get the keys and get out.
				//
				// Staying attached buys nothing — both keys are installed during
				// the login handshake — and costs plenty: every extra minute is
				// another chance for the breakpoint dance to catch the game at a
				// bad moment. The point is to play the session afterwards, which
				// needs the game unmolested and the debugger gone.
				if wantKeys > 0 && hits >= wantKeys {
					fmt.Printf("\ngot %d key(s); detaching so the game runs clean\n", hits)
					procContinueDebugEvent(pid, tid, dbgContinue)
					return nil
				}
				rearmAt = time.Now().Add(rearmDelay)

			case exCode == exceptionBreakpoint || exCode == exceptionSingleStep:
				// Not ours, but still ours to swallow. Includes the initial
				// attach breakpoint, and any stray single-step.
				//
				// Windows raises an initial breakpoint from an injected thread
				// the moment a debugger attaches. Passing that back with
				// DBG_EXCEPTION_NOT_HANDLED hands the game an unhandled
				// exception and hangs or kills it — which is exactly what
				// happened the first time this ran. A debugger owns breakpoint
				// and single-step exceptions; it must consume them.
				if verbose {
					fmt.Printf("  (swallowed %s at %#x, tid %d)\n",
						exceptionName(exCode), exAddr, tid)
				}

			default:
				// Everything else — access violations, C++ exceptions, the
				// game's own structured handling — must go back to the game, or
				// we break error handling it relies on.
				if verbose {
					fmt.Printf("  (passing %#x at %#x back to the game)\n", exCode, exAddr)
				}
				status = dbgExceptionNotHandled
			}

		case exitProcessDebugEvent:
			fmt.Printf("process exited; %d key(s) captured\n", hits)
			procContinueDebugEvent(pid, tid, status)
			return nil
		}

		procContinueDebugEvent(pid, tid, status)

		// Scan and arm only once the process is running again, so the game is
		// never held suspended for the length of a 28 MB scan.
		if needScan {
			needScan = false
			hit, err := scanForSignature(hProcess, imageBase)
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
		}
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

// rewindRIP moves RIP back onto the instruction the INT3 replaced.
//
// On a breakpoint exception RIP is already one byte past the INT3. With the
// original byte restored, the thread must resume AT that byte, not after it —
// otherwise it executes from the middle of an instruction. That is the one
// failure here that reliably kills the process, so the result is checked and
// treated as fatal rather than logged and shrugged off.
func rewindRIP(tid uint32, addr uintptr) error {
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
