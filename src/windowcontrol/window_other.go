//go:build !windows

package windowcontrol

import "time"
const Title = "Rev Visión | Historia Clínica"
func Exists() bool {return false}
func ActivateExisting() bool {return false}
func WaitForExisting(timeout time.Duration) bool {return false}
