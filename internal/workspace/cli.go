// Package workspace is the shared core every KRn feature builds on: repository
// discovery, repository-private storage under .git/krn, bounded output, and
// metrics.
package workspace

import "flag"

const Version = "0.3.0"

func FlagSet(name string) *flag.FlagSet { return flag.NewFlagSet(name, flag.ContinueOnError) }
