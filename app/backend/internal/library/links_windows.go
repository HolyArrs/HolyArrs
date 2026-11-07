package library

import "syscall"

// linkCount is the number of hard links to the file at p.
func linkCount(p string) (uint64, error) {
	name, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return 0, err
	}
	h, err := syscall.CreateFile(name, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.CloseHandle(h)
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &d); err != nil {
		return 0, err
	}
	return uint64(d.NumberOfLinks), nil
}
