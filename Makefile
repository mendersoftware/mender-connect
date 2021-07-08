DESTDIR ?=
prefix ?= /usr
bindir=$(prefix)/bin
datadir ?= $(prefix)/share
sysconfdir ?= /etc
systemd_unitdir ?= /lib/systemd

GO ?= go
GOFMT ?= gofmt
GOCYCLO ?= gocyclo
V ?=
PKGS = $(shell go list ./...)
PKGFILES = $(shell find . \( -path ./vendor \) -prune \
		-o -type f -name '*.go' -print)
PKGFILES_notest = $(shell echo $(PKGFILES) | tr ' ' '\n' | grep -v '_test.go' )
GOCYCLO_LIMIT ?= 15

TOOLS = \
	github.com/fzipp/gocyclo                     \
	gitlab.com/opennota/check/cmd/varcheck       \
	github.com/mendersoftware/deadcode           \
	github.com/mendersoftware/gobinarycoverage

VERSION = $(shell git describe --tags --dirty --exact-match 2>/dev/null || git rev-parse --short HEAD)

GO_LDFLAGS = \
	-ldflags "-X github.com/mendersoftware/mender-connect/config.Version=$(VERSION)"

ifeq ($(V),1)
BUILDV = -v
endif

TAGS =
ifeq ($(LOCAL),1)
TAGS += local
endif

ifneq ($(TAGS),)
BUILDTAGS = -tags '$(TAGS)'
endif

build: mender-connect

clean:
	@$(GO) clean
	@-rm -f coverage.txt

mender-connect: $(PKGFILES)
	@$(GO) build $(GO_LDFLAGS) $(BUILDV) $(BUILDTAGS)

install: install-bin install-systemd

install-bin: mender-connect
	@install -m 755 -d $(bindir)
	@install -m 755 mender-connect $(bindir)/

install-conf:
	@install -m 755 -d $(DESTDIR)$(sysconfdir)/mender
	@install -m 600 examples/mender-connect.conf $(DESTDIR)$(sysconfdir)/mender/

install-systemd:
	@install -m 755 -d $(DESTDIR)$(systemd_unitdir)/system
	@install -m 0644 support/mender-connect.service $(DESTDIR)$(systemd_unitdir)/system/

uninstall: uninstall-bin uninstall-systemd

uninstall-bin:
	@rm -f $(bindir)/mender-connect
	@-rmdir -p $(bindir)

uninstall-systemd:
	@rm -f $(DESTDIR)$(systemd_unitdir)/system/mender-connect.service
	@-rmdir -p $(DESTDIR)$(systemd_unitdir)/system

check: test extracheck

test:
	@$(GO) test $(BUILDV) $(PKGS)

extracheck: gofmt govet godeadcode govarcheck gocyclo
	@echo "All extra-checks passed!"

gofmt:
	@echo "-- checking if code is gofmt'ed"
	@if [ -n "$$($(GOFMT) -d $(PKGFILES))" ]; then \
		"$$($(GOFMT) -d $(PKGFILES))" \
		echo "-- gofmt check failed"; \
		/bin/false; \
	fi

govet:
	@echo "-- checking with govet"
	@$(GO) vet -unsafeptr=false

godeadcode:
	@echo "-- checking for dead code"
	@deadcode -ignore version.go:Version

govarcheck:
	@echo "-- checking with varcheck"
	@varcheck ./...

gocyclo:
	@echo "-- checking cyclometric complexity > $(GOCYCLO_LIMIT)"
	@$(GOCYCLO) -over $(GOCYCLO_LIMIT) $(PKGFILES_notest)

cover: coverage
	@$(GO) tool cover -func=coverage.txt

htmlcover: coverage
	@$(GO) tool cover -html=coverage.txt

coverage:
	@rm -f coverage.txt
	@$(GO) test -coverprofile=coverage-tmp.txt -coverpkg=./... ./...
	@if [ -f coverage-missing-subtests.txt ]; then \
		echo 'mode: set' > coverage.txt; \
		cat coverage-tmp.txt coverage-missing-subtests.txt | grep -v 'mode: set' >> coverage.txt; \
	else \
		mv coverage-tmp.txt coverage.txt; \
	fi
	@rm -f coverage-tmp.txt coverage-missing-subtests.txt

.PHONY: build
.PHONY: clean
.PHONY: get-tools
.PHONY: test
.PHONY: check
.PHONY: extracheck
.PHONY: cover
.PHONY: htmlcover
.PHONY: coverage
.PHONY: install
.PHONY: install-bin
.PHONY: install-systemd
.PHONY: uninstall
.PHONY: uninstall-bin
.PHONY: uninstall-systemd
