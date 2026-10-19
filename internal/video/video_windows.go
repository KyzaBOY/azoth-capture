package video

import "errors"

// errLinuxOnly is returned on Windows, where video recording is not
// implemented; the window hides the video controls there.
var errLinuxOnly = errors.New("video recording is available on Linux only")

// Probe reports video as unavailable on Windows.
func Probe() Info { return Info{Reason: errLinuxOnly.Error()} }

// Recording exists on Windows only so the shared app code compiles; Start
// never returns one.
type Recording struct{}

// Start always fails on Windows.
func Start(string, Options, func(string)) (*Recording, error) { return nil, errLinuxOnly }

// Done never closes on Windows (no recording ever runs).
func (*Recording) Done() <-chan struct{} { return nil }

// Err is always nil on Windows.
func (*Recording) Err() error { return nil }

// Stop does nothing on Windows.
func (*Recording) Stop() (Clip, error) { return Clip{}, errLinuxOnly }

// PickWindow always fails on Windows.
func PickWindow(string, func(string)) error { return errLinuxOnly }
