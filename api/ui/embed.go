package ui

import "embed"

//go:embed html/*.html static/*
var Files embed.FS
