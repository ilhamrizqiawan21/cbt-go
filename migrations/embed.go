// Package migrations menyediakan filesystem embedded untuk berkas migrasi SQL.
package migrations

import "embed"

// FS menampung semua berkas *.sql di folder ini untuk dibaca oleh goose.
//
//go:embed *.sql
var FS embed.FS
