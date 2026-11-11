package sync

import "runtime"

// dirIdentity — what tells one folder from another made later at the same
// path (a repository cloned again, a folder removed and made again, a disk
// mounted over it): the file number and the time the folder was created,
// as a string; "" when the folder is not there, is a link, or this system
// cannot tell (then the path decides, with the limits of bases.go).
//
// A file number alone is not enough: ext4 hands the number of a removed
// folder to the next one made, at once. Hence the creation time — statx
// on Linux, the birth time on macOS; on Windows the volume and the file id
// of NTFS and ReFS, which carries a sequence number that changes when the
// number is reused, with the creation time. The device number is not part
// of it on Linux and macOS: it changes across reboots on btrfs subvolumes,
// ZFS datasets and with the order disks are found in.
func dirIdentity(path string) string { return dirIdentityFunc(path) }

// dirIdentityFunc — tests stand in a file system that cannot tell.
var dirIdentityFunc = osDirIdentity

// osName — runtime.GOOS; tests stand in another system.
var osName = runtime.GOOS
