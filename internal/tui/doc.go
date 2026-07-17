// Package tui is the Bubble Tea terminal UI over the client.
//
// It renders an issue list, an issue detail view (documents, comments, and
// linked transcripts), and forms to create, edit/close, and create child
// issues. Every interaction reaches the daemon exclusively through the client
// package, per the architecture rule that tui may import only domain and
// client.
package tui
