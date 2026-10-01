// Package stdlib is jaala's standard vocabulary: the predicates every host gets unless it composes
// its own set. Strings registers the string tests under str (str.contains, str.prefix, str.suffix,
// str.glob, str.match), Absent registers absent at the root, and Register registers both.
//
// CompileGlob and CompilePattern are the compilers behind str.glob and str.match, exported so Go code
// that must match the same patterns (agni's profiles match nets to signals both in queries and in Go)
// shares one translation rather than keeping a twin that could drift.
//
// stdlib imports ns and never an engine, so a host's fact layer that may not import one can still
// register the standard predicates. The engine, jaala/datalog, does not import stdlib either.
package stdlib
