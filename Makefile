DST = /usr/local/bin

msmanager: msmanager.go util.go
	go build msmanager.go util.go

install: msmanager
	cp msmanager ${DST}

uninstall:
	rm ${DST}/msmanager

.PHONY: install uninstall
