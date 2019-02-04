GOARCH=amd64 go build -ldflags "-s" -o adlg_win64.exe *.go
REM GOARCH=386 go build -ldflags "-s" -o adlg_win32.exe *.go