//go:build windows

// memscan searches a running process's memory for a byte pattern and prints the
// surrounding bytes.
//
// Built because RPCS3's own memory-viewer search kept returning nothing, even for
// strings that are certainly resident — which left every negative result
// worthless, since we could not tell "not there" from "search not working". A
// scanner we control has a positive control we can trust.
//
// RPCS3 is an ordinary Windows process and the emulated PS3's memory lives inside
// its address space, so this finds guest data as readily as host data.
//
// It is strictly READ-ONLY: OpenProcess with read rights, VirtualQueryEx to walk
// the regions, ReadProcessMemory to look. No debugger attach, no breakpoints,
// nothing written. It cannot hang or crash the target the way keydump could.
//
// Usage:
//
//	memscan.exe -proc rpcs3.exe -hex 0045007300740075007300200046006c00610073006b
//	memscan.exe -proc rpcs3.exe -utf16be "Estus Flask"
//	memscan.exe -proc rpcs3.exe -ascii "regulation.fmg" -context 96
//
// -utf16be is the usual one for PS3 game text: the console is big-endian and its
// FMG strings are UTF-16BE.
package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess      = kernel32.NewProc("OpenProcess")
	procCloseHandle      = kernel32.NewProc("CloseHandle")
	procReadProcessMem   = kernel32.NewProc("ReadProcessMemory")
	procVirtualQueryEx   = kernel32.NewProc("VirtualQueryEx")
	procCreateToolhelp32 = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW  = kernel32.NewProc("Process32FirstW")
	procProcess32NextW   = kernel32.NewProc("Process32NextW")
)

const (
	processQueryInformation = 0x0400
	processVMRead           = 0x0010

	memCommit    = 0x1000
	pageNoAccess = 0x01
	pageGuard    = 0x100
)

// MEMORY_BASIC_INFORMATION, x64 layout.
type memInfo struct {
	BaseAddress       uintptr
	AllocationBase    uintptr
	AllocationProtect uint32
	_                 uint32
	RegionSize        uintptr
	State             uint32
	Protect           uint32
	Type              uint32
	_                 uint32
}

func main() {
	name := flag.String("proc", "rpcs3.exe", "process name to search")
	pid := flag.Int("pid", 0, "process id (overrides -proc)")
	hexPat := flag.String("hex", "", "pattern as hex bytes")
	utf16be := flag.String("utf16be", "", "pattern as a UTF-16BE string (PS3 text)")
	utf16le := flag.String("utf16le", "", "pattern as a UTF-16LE string")
	ascii := flag.String("ascii", "", "pattern as ASCII")
	context := flag.Int("context", 64, "bytes of context to print around each hit")
	max := flag.Int("max", 20, "stop after this many hits")
	flag.Parse()

	var pat []byte
	var err error
	switch {
	case *hexPat != "":
		pat, err = hex.DecodeString(strings.ReplaceAll(*hexPat, " ", ""))
		if err != nil {
			fatal("-hex is not valid hex: %v", err)
		}
	case *utf16be != "":
		pat = encodeUTF16(*utf16be, true)
	case *utf16le != "":
		pat = encodeUTF16(*utf16le, false)
	case *ascii != "":
		pat = []byte(*ascii)
	default:
		fatal("give one of -hex, -utf16be, -utf16le or -ascii")
	}
	if len(pat) == 0 {
		fatal("empty pattern")
	}

	target := uint32(*pid)
	if target == 0 {
		target, err = findProcess(*name)
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("found %s, pid %d\n", *name, target)
	}

	h, _, errno := procOpenProcess.Call(processQueryInformation|processVMRead, 0, uintptr(target))
	if h == 0 {
		fatal("OpenProcess(%d): %v (try running as Administrator)", target, errno)
	}
	defer procCloseHandle.Call(h)

	fmt.Printf("searching for %d bytes: %s\n\n", len(pat), hex.EncodeToString(pat))

	var scanned uint64
	var regions, hits int
	buf := make([]byte, 4<<20)

	for addr := uintptr(0); ; {
		var mi memInfo
		r, _, _ := procVirtualQueryEx.Call(h, addr, uintptr(unsafe.Pointer(&mi)), unsafe.Sizeof(mi))
		if r == 0 {
			break
		}
		next := mi.BaseAddress + mi.RegionSize
		if next <= addr {
			break
		}

		readable := mi.State == memCommit &&
			mi.Protect&pageNoAccess == 0 &&
			mi.Protect&pageGuard == 0
		if readable {
			regions++
			for off := uintptr(0); off < mi.RegionSize; {
				n := uintptr(len(buf))
				if off+n > mi.RegionSize {
					n = mi.RegionSize - off
				}
				var read uintptr
				ok, _, _ := procReadProcessMem.Call(h, mi.BaseAddress+off,
					uintptr(unsafe.Pointer(&buf[0])), n, uintptr(unsafe.Pointer(&read)))
				if ok != 0 && read > 0 {
					scanned += uint64(read)
					chunk := buf[:read]
					for i := 0; i+len(pat) <= len(chunk); i++ {
						if chunk[i] != pat[0] {
							continue
						}
						if string(chunk[i:i+len(pat)]) != string(pat) {
							continue
						}
						hits++
						at := mi.BaseAddress + off + uintptr(i)
						fmt.Printf("HIT %d at %#x  (region %#x, protect %#x)\n", hits, at, mi.BaseAddress, mi.Protect)
						printContext(chunk, i, at, *context)
						if hits >= *max {
							fmt.Printf("\nstopping at %d hits\n", hits)
							return
						}
					}
				}
				// Overlap so a match spanning a chunk boundary is not missed.
				if n < uintptr(len(pat)) {
					break
				}
				off += n - uintptr(len(pat)) + 1
			}
		}
		addr = next
	}

	fmt.Printf("\nscanned %.1f MB across %d readable regions, %d hit(s)\n",
		float64(scanned)/(1<<20), regions, hits)
	if hits == 0 {
		fmt.Println("\nNo hits. If a string you KNOW is on screen also returns nothing,")
		fmt.Println("the encoding assumption is wrong — try -utf16le or -ascii.")
	}
}

func printContext(chunk []byte, i int, at uintptr, n int) {
	lo := i - n
	if lo < 0 {
		lo = 0
	}
	hi := i + n
	if hi > len(chunk) {
		hi = len(chunk)
	}
	fmt.Printf("    bytes %#x..%#x:\n", at-uintptr(i-lo), at+uintptr(hi-i))
	fmt.Printf("    %s\n", hex.EncodeToString(chunk[lo:hi]))
	// PS3 text is UTF-16BE; rendering the window that way often shows the whole
	// surrounding string, which is the point of running this at all.
	fmt.Printf("    as UTF-16BE: %q\n", decodeUTF16BE(chunk[lo:hi]))
}

func encodeUTF16(s string, bigEndian bool) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		if r > 0xFFFF {
			continue
		}
		if bigEndian {
			out = append(out, byte(r>>8), byte(r))
		} else {
			out = append(out, byte(r), byte(r>>8))
		}
	}
	return out
}

func decodeUTF16BE(b []byte) string {
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.BigEndian.Uint16(b[i:])
		if c >= 0x20 && c < 0x7f {
			sb.WriteByte(byte(c))
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}

func findProcess(name string) (uint32, error) {
	const th32csSnapProcess = 0x2
	snap, _, err := procCreateToolhelp32.Call(th32csSnapProcess, 0)
	if snap == 0 || snap == uintptr(^uintptr(0)) {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot: %v", err)
	}
	defer procCloseHandle.Call(snap)

	const entrySize = 568
	entry := make([]byte, entrySize)
	binary.LittleEndian.PutUint32(entry[0:], entrySize)

	r, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry[0])))
	for r != 0 {
		pid := binary.LittleEndian.Uint32(entry[8:])
		var u []uint16
		for i := 44; i+1 < len(entry); i += 2 {
			c := binary.LittleEndian.Uint16(entry[i:])
			if c == 0 {
				break
			}
			u = append(u, c)
		}
		if strings.EqualFold(syscall.UTF16ToString(u), name) {
			return pid, nil
		}
		binary.LittleEndian.PutUint32(entry[0:], entrySize)
		r, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry[0])))
	}
	return 0, fmt.Errorf("no process named %q is running", name)
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "memscan: "+format+"\n", a...)
	os.Exit(1)
}
