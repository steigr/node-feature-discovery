GO=go

DESTDIR ?= ""
SYSCONFDIR ?= /etc
INSTALLDIR = $(DESTDIR)$(SYSCONFDIR)/kubernetes/node-feature-discovery/source.d/

.PHONY: all install

all: cpu

cpu: cpu.go
	$(GO) build ./cpu.go

install: cpu
	install -d $(INSTALLDIR)
	install -t $(INSTALLDIR) cpu

