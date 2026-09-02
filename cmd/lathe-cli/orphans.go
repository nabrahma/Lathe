package main

import "time"

// defaultOrphanAge is how old a leftover workspace must be before it is
// assumed to belong to a crashed run rather than to another instance that is
// still working.
const defaultOrphanAge = 6 * time.Hour
