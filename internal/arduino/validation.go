package arduino

import (
	"regexp"
	"strings"
)

var versionPattern = regexp.MustCompile(`^[A-Za-z0-9_.+-]+$`)
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+$`)
var fqbnPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+(:[A-Za-z0-9_.=-]+(,[A-Za-z0-9_.=-]+)*)?$`)

func ValidCore(id string) bool         { return idPattern.MatchString(id) }
func ValidFQBN(id string) bool         { return fqbnPattern.MatchString(id) }
func ValidVersion(version string) bool { return version == "" || versionPattern.MatchString(version) }
func ValidLibrary(name string) bool { return len(name) > 0 && len(name) <= 256 && !strings.HasPrefix(name, "-") && !strings.ContainsAny(name, "\r\n\x00@") }
