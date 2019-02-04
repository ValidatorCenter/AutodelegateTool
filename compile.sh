#!/bin/sh

GOARCH=amd64 go build -ldflags "-s" -o adlg_lin64 *.go