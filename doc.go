// Package sup provides a simple utility that allows supervision of other processes called child processes.
// A child process can be a simple goroutine or another supervisor.
// Supervisors build hierarchical process structures, a supervision tree.
// They're very well suited to structure fault-tolerant application.
package sup
