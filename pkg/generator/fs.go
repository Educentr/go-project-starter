package generator

import "embed"

//go:embed templates/*
var templates embed.FS

//go:embed embedded/disclaimer.txt
var disclaimer string

//go:embed embedded/splitter.txt
var splitter string
