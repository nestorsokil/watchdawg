module watchdawg

go 1.24

require (
	github.com/robfig/cron/v3 v3.0.1
	github.com/segmentio/kafka-go v0.4.50
	go.starlark.net v0.0.0-20251109183026-be02852a5e1f
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
)

replace (
	github.com/klauspost/compress v1.15.9 => ./stubs/compress
	github.com/pierrec/lz4/v4 v4.1.15 => ./stubs/lz4v4
)
