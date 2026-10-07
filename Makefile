#
# Makefile for logagent, a Go module.
#
# This file is a convenience. The module builds and installs with plain Go:
#
#     go build ./cmd/...
#     go install github.com/caltechlibrary/logagent/cmd/logagent@latest
#
# Required to build:    Go (current stable) and Pandoc (man pages), plus the
#                       git, make, grep, cut, sed and zip the operating system
#                       supplies.
# Also required to run
# release.bash:         gh and jq. (release.ps1 needs gh only.)
#
PROJECT = logagent

GIT_GROUP = caltechlibrary

PROGRAMS = logagent

VERSION = $(shell grep '"version":' codemeta.json | cut -d\" -f 4)

BRANCH = $(shell git branch --show-current)

RELEASE_DATE = $(shell date +%Y-%m-%d)

RELEASE_HASH = $(shell git log --pretty=format:%h -n 1)

OS = $(shell uname)

EXT =
ifeq ($(OS), Windows)
	EXT = .exe
endif

PREFIX = $(HOME)

ifneq ($(prefix),)
	PREFIX = $(prefix)
endif

# One entry per release platform: name:GOOS:GOARCH
PLATFORMS = Linux-x86_64:linux:amd64 Linux-aarch64:linux:arm64 \
	macOS-x86_64:darwin:amd64 macOS-arm64:darwin:arm64 \
	Windows-x86_64:windows:amd64 Windows-arm64:windows:arm64

build: version.go bin docs man

bin: .FORCE
	@mkdir -p bin
	go build -o bin/ ./cmd/...

# version.go holds Version, ReleaseDate and ReleaseHash. This rule rewrites
# them from codemeta.json, today's date and the current git hash, and runs
# only when codemeta.json is newer (use make -B version.go to force it, as
# release does). It stops without writing if codemeta.json does not have
# exactly one "version" line.
version.go: codemeta.json
	@if [ "$$(grep -c '"version":' codemeta.json)" != "1" ]; then \
		echo 'error: codemeta.json must contain exactly one "version": line' >&2; exit 1; fi
	@if [ -z "$(VERSION)" ]; then echo 'error: no version found in codemeta.json' >&2; exit 1; fi
	@sed -e 's/^\(	Version = \).*/\1"$(VERSION)"/' \
	     -e 's/^\(	ReleaseDate = \).*/\1"$(RELEASE_DATE)"/' \
	     -e 's/^\(	ReleaseHash = \).*/\1"$(RELEASE_HASH)"/' version.go >version.go.tmp
	@mv version.go.tmp version.go
	@echo "version.go: $(VERSION) $(RELEASE_DATE) $(RELEASE_HASH)"

# docs/ holds one Markdown file per manual page, generated from the built
# program: NAME.CHAPTER.md, for example logagent-check.1.md. The list of pages
# comes from the program itself (logagent help --list).
docs: bin .FORCE
	@mkdir -p docs
	@for PAGE in $$(./bin/logagent help --list); do ./bin/logagent help $$PAGE >docs/$$PAGE.md; done

# man renders each docs/NAME.CHAPTER.md into man/manCHAPTER/NAME.CHAPTER.
man: docs .FORCE
	@for FNAME in docs/*.?.md; do \
		PAGE=$$(basename $$FNAME .md); CHAPTER=$${PAGE##*.}; \
		mkdir -p man/man$$CHAPTER; \
		pandoc $$FNAME --from markdown --to man -s >man/man$$CHAPTER/$$PAGE; \
	done

test: .FORCE
	go vet ./...
	go test ./...

status:
	git status

save:
	@if [ "$(msg)" != "" ]; then git commit -am "$(msg)"; else git commit -am "Quick Save"; fi
	git push origin $(BRANCH)

refresh:
	git fetch origin
	git pull origin $(BRANCH)

clean:
	@if [ -d bin ]; then rm -fR bin; fi
	@if [ -d dist ]; then rm -fR dist; fi
	@if [ -d man ]; then rm -fR man; fi
	@if [ -f version.go.tmp ]; then rm version.go.tmp; fi

install: build
	@echo "Installing programs in $(PREFIX)/bin"
	@mkdir -p "$(PREFIX)/bin"
	@for FNAME in $(PROGRAMS); do if [ -f "./bin/$${FNAME}$(EXT)" ]; then cp -v "./bin/$${FNAME}$(EXT)" "$(PREFIX)/bin/$${FNAME}$(EXT)"; fi; done
	@echo ""
	@echo "Make sure $(PREFIX)/bin is in your PATH"
	@echo "Installing man pages in $(PREFIX)/man"
	@for FNAME in man/man*/*; do \
		CHAPTER=$$(basename $$(dirname $$FNAME)); \
		mkdir -p "$(PREFIX)/man/$$CHAPTER"; \
		cp -v "$$FNAME" "$(PREFIX)/man/$$CHAPTER/"; \
	done
	@echo ""
	@echo "Make sure $(PREFIX)/man is in your MANPATH"

uninstall: .FORCE
	@echo "Removing programs in $(PREFIX)/bin"
	@for FNAME in $(PROGRAMS); do if [ -f "$(PREFIX)/bin/$${FNAME}$(EXT)" ]; then rm -v "$(PREFIX)/bin/$${FNAME}$(EXT)"; fi; done
	@echo "Removing man pages in $(PREFIX)/man"
	@for FNAME in docs/*.?.md; do \
		PAGE=$$(basename $$FNAME .md); CHAPTER=$${PAGE##*.}; \
		if [ -f "$(PREFIX)/man/man$$CHAPTER/$$PAGE" ]; then rm -v "$(PREFIX)/man/man$$CHAPTER/$$PAGE"; fi; \
	done

# dist builds one zip per platform in PLATFORMS. It starts from an empty dist/.
dist: build .FORCE
	@rm -fR dist
	@mkdir -p dist
	@for ENTRY in $(PLATFORMS); do \
		NAME=$$(echo $$ENTRY | cut -d: -f1); GOOS=$$(echo $$ENTRY | cut -d: -f2); GOARCH=$$(echo $$ENTRY | cut -d: -f3); \
		SUFFIX=""; if [ "$$GOOS" = "windows" ]; then SUFFIX=".exe"; fi; \
		rm -fR dist/stage; mkdir -p dist/stage/bin; \
		for FNAME in $(PROGRAMS); do \
			env GOOS=$$GOOS GOARCH=$$GOARCH go build -o "dist/stage/bin/$${FNAME}$$SUFFIX" ./cmd/$$FNAME || exit 1; \
		done; \
		cp LICENSE codemeta.json CITATION.cff README.md INSTALL.md dist/stage/; \
		cp -R docs examples man dist/stage/; \
		(cd dist/stage && zip -qr ../$(PROJECT)-v$(VERSION)-$$NAME.zip .) || exit 1; \
		echo "dist/$(PROJECT)-v$(VERSION)-$$NAME.zip"; \
	done
	@rm -fR dist/stage

release-version: .FORCE
	@$(MAKE) -B version.go

release: clean release-version test dist
	@printf "\nready to run\n\n\t./release.bash   (or release.ps1 on Windows)\n\n"

.FORCE:
