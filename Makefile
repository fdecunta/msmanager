DST = /usr/local/bin

msmanager: msmanager.go
	go build msmanager.go

install: msmanager
	cp msmanager ${DST}

uninstall:
	rm ${DST}/msmanager

.PHONY: install uninstall
