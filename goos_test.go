// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import "runtime"

// isWindows is asked by tests that run everywhere, so it is not declared in
// the per-system test files: a system that is neither unix nor windows
// (plan9, js) would have no declaration at all.
const isWindows = runtime.GOOS == "windows"
