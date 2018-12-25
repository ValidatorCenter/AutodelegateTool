GOARCH=amd64 go build -ldflags "-s" -o adlg_win64.exe deleg.go
GOARCH=386 go build -ldflags "-s" -o adlg_win32.exe deleg.go