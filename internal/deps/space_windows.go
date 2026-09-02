package deps

import "golang.org/x/sys/windows"

// freeSpace reports the bytes available to this user on the volume holding dir.
func freeSpace(dir string) (int64, error) {
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var freeToCaller, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(path, &freeToCaller, &total, &totalFree); err != nil {
		return 0, err
	}
	return int64(freeToCaller), nil
}
