//go:build windows

package pickdir

import (
	"context"
	"syscall"
	"unsafe"
)

// Direct Win32 IFileOpenDialog (modern folder picker, Vista+) via COM.
// No PowerShell, no .NET — no transient console window is spawned. The
// dialog runs in-process on the calling goroutine. Vtable dispatch uses
// typed method structs (the go-ole idiom) so go vet's unsafeptr
// analyzer accepts every Pointer/uintptr conversion below.

var (
	modole32 = syscall.NewLazyDLL("ole32.dll")

	procCoInitializeEx   = modole32.NewProc("CoInitializeEx")
	procCoUninitialize   = modole32.NewProc("CoUninitialize")
	procCoCreateInstance = modole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = modole32.NewProc("CoTaskMemFree")
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidFileOpenDialog = guid{
		0xDC1C5A9C, 0xE88A, 0x4DDE,
		[8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7},
	}
	iidIFileOpenDialog = guid{
		0xD57C7288, 0xD4AD, 0x4768,
		[8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60},
	}
)

const (
	clsctxInprocServer      = 0x1
	coinitApartmentThreaded = 0x2

	fosPickFolders     uintptr = 0x20
	fosForceFileSystem uintptr = 0x40
	fosPathMustExist   uintptr = 0x800

	sigdnFilesysPath uintptr = 0x80058000

	hresultOK      uintptr = 0
	hresultCancel          = uintptr(0x800704C7)
	rpcEChangedMode        = uintptr(0x80010106)
)

// iFileDialogVtbl is the IFileDialog vtable. Methods are accessed by
// named field, not by numeric index, so the access pattern is
// indistinguishable from any ordinary struct read as far as vet is
// concerned. The full vtable layout is in shobjidl_core.h.
type iFileDialogVtbl struct {
	queryInterface  uintptr // 0
	addRef          uintptr // 1
	release         uintptr // 2
	show            uintptr // 3
	setFileTypes    uintptr // 4
	setFileTypeIdx  uintptr // 5
	getFileTypeIdx  uintptr // 6
	advise          uintptr // 7
	unadvise        uintptr // 8
	setOptions      uintptr // 9
	getOptions      uintptr // 10
	setDefaultFldr  uintptr // 11
	setFolder       uintptr // 12
	getFolder       uintptr // 13
	getCurSelection uintptr // 14
	setFileName     uintptr // 15
	getFileName     uintptr // 16
	setTitle        uintptr // 17
	setOkLabel      uintptr // 18
	setFileNameLbl  uintptr // 19
	getResult       uintptr // 20
	addPlace        uintptr // 21
	setDefaultExt   uintptr // 22
	close_          uintptr // 23
	setClientGuid   uintptr // 24
	clearClientData uintptr // 25
	setFilter       uintptr // 26
}

type iFileDialog struct{ vtbl *iFileDialogVtbl }

// iShellItemVtbl is the IShellItem vtable. GetDisplayName is the only
// method we use.
type iShellItemVtbl struct {
	queryInterface uintptr // 0
	addRef         uintptr // 1
	release        uintptr // 2
	bindToHandler  uintptr // 3
	getParent      uintptr // 4
	getDisplayName uintptr // 5
	getAttributes  uintptr // 6
	compare        uintptr // 7
}

type iShellItem struct{ vtbl *iShellItemVtbl }

// Pick opens the OS folder dialog and returns the chosen path.
// Cancelled dialogs return ("", nil).
func Pick(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	hrInit, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	ownsInit := hrInit == hresultOK
	if hrInit < 0 && hrInit != rpcEChangedMode {
		return "", syscall.Errno(hrInit)
	}
	defer func() {
		if ownsInit {
			procCoUninitialize.Call()
		}
	}()

	var dlg *iFileDialog
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&dlg)),
	)
	if hr < 0 {
		return "", syscall.Errno(hr)
	}
	defer func() { _, _, _ = syscall.SyscallN(dlg.vtbl.release, uintptr(unsafe.Pointer(dlg))) }()

	curOpts, _, _ := syscall.SyscallN(dlg.vtbl.getOptions, uintptr(unsafe.Pointer(dlg)))
	newOpts := curOpts | fosPickFolders | fosForceFileSystem | fosPathMustExist
	if hrRet, _, _ := syscall.SyscallN(dlg.vtbl.setOptions, uintptr(unsafe.Pointer(dlg)), newOpts); int32(hrRet) < 0 {
		return "", syscall.Errno(hrRet)
	}

	title, _ := syscall.UTF16PtrFromString("Select game directory")
	if hrRet, _, _ := syscall.SyscallN(dlg.vtbl.setTitle, uintptr(unsafe.Pointer(dlg)), uintptr(unsafe.Pointer(title))); int32(hrRet) < 0 {
		return "", syscall.Errno(hrRet)
	}

	hrShow, _, _ := syscall.SyscallN(dlg.vtbl.show, uintptr(unsafe.Pointer(dlg)), 0)
	if hrShow == hresultCancel {
		return "", nil
	}
	if int32(hrShow) < 0 {
		return "", syscall.Errno(hrShow)
	}

	var item *iShellItem
	if hrRet, _, _ := syscall.SyscallN(dlg.vtbl.getResult, uintptr(unsafe.Pointer(dlg)), uintptr(unsafe.Pointer(&item))); int32(hrRet) < 0 {
		return "", syscall.Errno(hrRet)
	}
	if item == nil {
		return "", nil
	}
	defer func() { _, _, _ = syscall.SyscallN(item.vtbl.release, uintptr(unsafe.Pointer(item))) }()

	var pathPtr *uint16
	if hrRet, _, _ := syscall.SyscallN(item.vtbl.getDisplayName, uintptr(unsafe.Pointer(item)), sigdnFilesysPath, uintptr(unsafe.Pointer(&pathPtr))); int32(hrRet) < 0 {
		return "", syscall.Errno(hrRet)
	}
	if pathPtr == nil {
		return "", nil
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(pathPtr)))

	return utf16PtrToString(pathPtr), nil
}

func utf16PtrToString(ptr *uint16) string {
	if ptr == nil {
		return ""
	}
	var s []uint16
	for i := 0; ; i++ {
		c := *(*uint16)(unsafe.Add(unsafe.Pointer(ptr), uintptr(i)*2))
		if c == 0 {
			break
		}
		s = append(s, c)
	}
	return syscall.UTF16ToString(s)
}
