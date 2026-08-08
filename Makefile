
.PHONY: build build-win build-all build-local build-local-test run run-fake-review \
	dev-ui design-ui bump release release-internal release-gh clean test test-go \
	test-simulator test-hooks-worktree test-hooks-claude test-hooks-global \
	test-install-local test-plugin-bootstrap test-plugin-hooks test-loop-prevention \
	test-powershell-smoke test-live-smoke \
	test-js testall test-pkg upload-secrets download-secrets \
	security-govulncheck security-govulncheck-json security-osv security-triage \
	security-gitleaks security-b2-audit security-b2-cleanup-plan \
	security-b2-cleanup-apply security-publish-release-manifest \
	security-secret-regression security-sbom security-sbom-cyclonedx \
	security-sbom-spdx security-sbom-validate release-notes-init \
	release-notes-check release-preflight check-status-doc copy-installers use-local-backend \
	use-livereview-backend

# Go parameters
GOENV=env -u GOROOT
GOCMD=$(GOENV) go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
BINARY_NAME=lrc
REQUIRED_GO_VERSION=$(shell awk '/^go /{print $$2; exit}' go.mod)
GOVULNCHECK_VERSION=v1.1.4
GOVULNCHECK_CMD=GOTOOLCHAIN=go$(REQUIRED_GO_VERSION) $(GOCMD) run -a golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
GH_REPO=HexmosTech/git-lrc
GH=/usr/bin/gh
ENV_VARS=B2_KEY_ID B2_APP_KEY B2_BUCKET_NAME B2_BUCKET_ID
SYFT_CMD=syft
SBOM_DIR=security_issues/sbom
SBOM_VERSION?=$(shell awk -F'"' '/^const appVersion/{print $$2; exit}' main.go)
SBOM_CDX=$(SBOM_DIR)/git-lrc-$(SBOM_VERSION)-cyclonedx.json
SBOM_SPDX=$(SBOM_DIR)/git-lrc-$(SBOM_VERSION)-spdx.json
RELEASE_NOTES_DIR=docs/releases
RELEASE_IMAGE_DIR=$(RELEASE_NOTES_DIR)/img
RELEASE_IMAGE_GUIDE=README.md
RELEASE_NOTES_TEMPLATE=$(RELEASE_NOTES_DIR)/_template.md
RELEASE_GH_SCRIPT=scripts/release_gh.py
RELEASE_NOTES_BRANCH=main
HEXMOS_HOMEPAGE_DIR=/home/shrsv/bin/hexmoshomepage
INSTALLER_COPY_TARGETS=$(HEXMOS_HOMEPAGE_DIR)/public/lrc-install.sh \
	$(HEXMOS_HOMEPAGE_DIR)/public/lrc-install.ps1 \
	$(HEXMOS_HOMEPAGE_DIR)/out/lrc-install.sh \
	$(HEXMOS_HOMEPAGE_DIR)/out/lrc-install.ps1

# Build lrc for the current platform
build:
	$(GOBUILD) -o $(BINARY_NAME) .

# Build lrc for Windows (amd64)
# Output: dist/windows/lrc.exe
build-win:
	@echo "🔨 Building lrc CLI for Windows (amd64)..."
	@mkdir -p dist/windows
	@GOOS=windows GOARCH=amd64 $(GOBUILD) -o dist/windows/$(BINARY_NAME).exe .
	@echo "✅ Built dist/windows/$(BINARY_NAME).exe"

# Build lrc for all platforms (linux/darwin/windows × amd64/arm64)
# Output: dist/<platform>/lrc[.exe] + SHA256SUMS
# Version is extracted from appVersion constant in main.go
build-all:
	@echo "🔨 Building lrc CLI for all platforms..."
	@python scripts/lrc_build.py -v build

# Build lrc locally for the current platform and install
build-local:
	@echo "🔨 Building lrc CLI locally (dirty tree allowed)..."
	@$(GOBUILD) -ldflags "-X main.version=$(SBOM_VERSION)-internal" -o /tmp/lrc .
	@mkdir -p $(HOME)/.local/bin
	@install -m 0755 /tmp/lrc $(HOME)/.local/bin/lrc
	@cp $(HOME)/.local/bin/lrc $(HOME)/.local/bin/git-lrc
	@echo "✅ Installed lrc and git-lrc to ~/.local/bin"
	@case ":$$PATH:" in *:$(HOME)/.local/bin:*) ;; *) echo "⚠️  ~/.local/bin is not in PATH. Run: source ~/.lrc/env" ;; esac

# Build lrc locally in fake-review mode for E2E testing (no AI calls)
build-local-test:
	@echo "🔨 Building lrc CLI locally in FAKE REVIEW mode..."
	@$(GOBUILD) -ldflags "-X main.reviewMode=fake" -o /tmp/lrc .
	@mkdir -p $(HOME)/.local/bin
	@install -m 0755 /tmp/lrc $(HOME)/.local/bin/lrc
	@cp $(HOME)/.local/bin/lrc $(HOME)/.local/bin/git-lrc
	@echo "✅ Installed fake-review lrc and git-lrc to ~/.local/bin"
	@echo "   Use WAIT=30s make run-fake-review (or set LRC_FAKE_REVIEW_WAIT)"
	@case ":$$PATH:" in *:$(HOME)/.local/bin:*) ;; *) echo "⚠️  ~/.local/bin is not in PATH. Run: source ~/.lrc/env" ;; esac

# Run the locally built lrc CLI (pass args via ARGS="--flag value")
run: build-local
	@echo "▶️ Running lrc CLI locally..."
	@lrc $(ARGS)

# Run fake review flow using fake-review build (defaults: WAIT=30s, TMP_REPO=/tmp/lrc-fake-review-repo)
run-fake-review: build-local-test
	@WAIT=$${WAIT:-30s} TMP_REPO=$${TMP_REPO:-/tmp/lrc-fake-review-repo} scripts/fake_review.sh $(ARGS)

# Run fake review with live JS reloading — edit files in internal/staticserve/static/, refresh browser
# No rebuild needed after JS changes: just edit and refresh the browser tab.
dev-ui: build-local-test
	@LRC_STATIC_DEV_DIR=$(CURDIR)/internal/staticserve/static \
	 WAIT=$${WAIT:-5s} \
	 TMP_REPO=$${TMP_REPO:-/tmp/lrc-fake-review-repo} \
	 scripts/fake_review.sh $(ARGS)

# Serve the review UI against captured real-review fixtures (big diff, real AI
# comments, real blast-radius data) for design iteration — edit files in
# internal/staticserve/static/, refresh browser. Fixtures live in
# tools/uidev/fixtures/ (re-capture with scripts/capture_design_fixture.sh).
design-ui:
	@go run ./tools/uidev $(if $(PORT),--port $(PORT),)

use-local-backend:
	@sed -i 's|^api_url = ".*"|api_url = "http://localhost:8888"|' $(HOME)/.lrc.toml
	@echo "✅ Switched to local backend (http://localhost:8888)"

use-livereview-backend:
	@sed -i 's|^api_url = ".*"|api_url = "https://livereview.hexmos.com"|' $(HOME)/.lrc.toml
	@echo "✅ Switched to livereview.hexmos.com"

copy-installers:
	@printf '%s\n' "This will overwrite the following installer files in $(HEXMOS_HOMEPAGE_DIR):"; \
	printf '  %s\n' $(INSTALLER_COPY_TARGETS); \
	printf 'Overwrite these files? [y/N]: '; \
	read ans; \
	if [ "$$ans" != "y" ] && [ "$$ans" != "Y" ]; then \
		echo "Skipped installer copy."; \
		exit 0; \
	fi; \
	install -m 0644 scripts/lrc-install.sh $(HEXMOS_HOMEPAGE_DIR)/public/lrc-install.sh; \
	install -m 0644 scripts/lrc-install.ps1 $(HEXMOS_HOMEPAGE_DIR)/public/lrc-install.ps1; \
	install -m 0644 scripts/lrc-install.sh $(HEXMOS_HOMEPAGE_DIR)/out/lrc-install.sh; \
	install -m 0644 scripts/lrc-install.ps1 $(HEXMOS_HOMEPAGE_DIR)/out/lrc-install.ps1; \
	echo "✅ Copied installers into $(HEXMOS_HOMEPAGE_DIR)"

# Bump lrc version by editing appVersion in main.go
# Prompts for version bump type (patch/minor/major)
bump:
	@echo "📝 Bumping lrc version..."
	@python3 scripts/lrc_build.py bump

# Build and upload lrc to Backblaze B2
release: check-status-doc
	@echo "🚀 Building and releasing lrc..."
	@python scripts/lrc_build.py -v release
	@version="$$(python3 $(RELEASE_GH_SCRIPT) --print-version)" || exit $$?; \
	notes="$(RELEASE_NOTES_DIR)/$$version.md"; \
	img_dir="$(RELEASE_IMAGE_DIR)/$$version"; \
	if [ -f "$$notes" ] || [ -d "$$img_dir" ]; then \
		echo "ℹ️  Release scaffold already exists:"; \
		echo "   Notes: $$notes"; \
		echo "   Images: $$img_dir"; \
		echo "   Next: edit the markdown, add media, then run make release-gh VERSION=$$version"; \
		exit 0; \
	fi; \
	printf "Create release markdown and image folder for %s? [y/N]: " "$$version"; \
	read ans; \
	if [ "$$ans" = "y" ] || [ "$$ans" = "Y" ]; then \
		$(MAKE) release-notes-init VERSION="$$version"; \
		echo "ℹ️  Edit $$notes"; \
		echo "ℹ️  Drop release images, GIFs, or local video files into $$img_dir"; \
		echo "ℹ️  Reference them in the markdown with ![alt](IMG:path/to/file.png)"; \
		echo "ℹ️  For a manual video reminder, keep <!-- VIDEO:demo.mp4 --> in a markdown comment."; \
		echo "ℹ️  IMG references publish from https://raw.githubusercontent.com/$(GH_REPO)/refs/heads/$(RELEASE_NOTES_BRANCH)/$(RELEASE_IMAGE_DIR)/$$version/path/to/file.png"; \
		echo "ℹ️  Publish when ready: make release-gh VERSION=$$version"; \
		echo "ℹ️  commit and push"; \
		echo "ℹ️  make release-gh"; \
	else \
		echo "ℹ️  Skipped release scaffold."; \
		echo "   Create it later with: make release-notes-init VERSION=$$version"; \
		echo "   Markdown path: $$notes"; \
		echo "   Image directory: $$img_dir"; \
		echo "   Publish when ready: make release-gh VERSION=$$version"; \
		echo "   commit and push"; \
		echo "   make release-gh"; \
	fi

# Build and upload an internal release of lrc using the same storage layout.
release-internal: check-status-doc
	@echo "🚀 Building and releasing internal lrc..."
	@python scripts/lrc_build.py -v release --channel internal
	@echo "ℹ️  Internal releases use a fixed pseudo-version and do not self-update"

# Optionally publish a GitHub release using markdown notes (no binary assets).
# VERSION is optional and auto-inferred by scripts/release_gh.py.
release-gh:
	@version="$$(python3 $(RELEASE_GH_SCRIPT) --print-version $(if $(VERSION),--version $(VERSION),))" || exit $$?; \
	$(MAKE) release-preflight VERSION="$$version"; \
	python3 $(RELEASE_GH_SCRIPT) --repo $(GH_REPO) --version "$$version"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf dist/ $(BINARY_NAME)
	@echo "✅ Clean complete"

# Run the full Go test baseline
test-go:
	$(GOTEST) -count=1 ./...

# Run simulator and nearby workflow-specific regressions
test-simulator:
	$(GOTEST) -count=1 ./internal/simulator
	# Keep the appcore compatibility spot-check explicit; these cases are called out in internal/simulator/README.md.
	$(GOTEST) -count=1 ./internal/appcore -run 'TestActionAllowedInPhase|TestValidateInteractiveDecisionRequest|TestReadCommitMessageFromRequest|TestPollReviewFakeCompletes|TestPollReviewFakeCancelled'

# Run deterministic Git/worktree hook regression harness
test-hooks-worktree: build-local
	@PATH="$(HOME)/.local/bin:$$PATH" bash tests/worktree-hooks.sh

# Run deterministic Claude hook regression harness
test-hooks-claude: build-local
	@PATH="$(HOME)/.local/bin:$$PATH" bash tests/claude-worktree-hooks.sh

# Run isolated local installer bootstrap validation against the local claude-lrc marketplace checkout.
test-install-local:
	@bash tests/installer-local.sh

# Run isolated plugin-first bootstrap validation against the local claude-lrc marketplace checkout.
test-plugin-bootstrap:
	@bash tests/plugin-bootstrap.sh

# Run isolated plugin-managed hook invocation validation against the local claude-lrc checkout.
test-plugin-hooks:
	@bash tests/plugin-hooks.sh

# Run isolated loop-prevention validation for both installer-first and plugin-first entry orders.
test-loop-prevention:
	@bash tests/loop-prevention.sh

# Run a narrow PowerShell smoke lane when PowerShell and a Windows host are available.
test-powershell-smoke:
	@bash tests/powershell-smoke.sh

# Run opt-in live smoke tests against published installer URLs and the pushed GitHub marketplace.
test-live-smoke:
	@LIVE_SMOKE="$(LIVE_SMOKE)" LIVE="$(LIVE)" bash tests/live-smoke.sh

# Run deterministic global hook lifecycle regression harness
test-hooks-global: build-local-test
	@PATH="$(HOME)/.local/bin:$$PATH" LRC_TEST_BIN="$(HOME)/.local/bin/lrc" bash tests/global-hooks.sh

# Run headless-safe Node UI state tests
test-js:
	node --test internal/staticserve/static/components/*.test.mjs

# Preserve the existing default test alias
test: test-go

# Run the current minimum-confidence deterministic lanes.
# test-go already covers the simulator package, so keep the aggregate target non-duplicative.
testall: test-go test-hooks-worktree test-hooks-claude test-hooks-global

# Run tests for a specific package (example: make test-pkg PKG=./internal/naming)
test-pkg:
	@if [ -z "$(PKG)" ]; then \
		echo "Usage: make test-pkg PKG=./path/to/package"; \
		exit 1; \
	fi
	$(GOTEST) -count=1 $(PKG)

# Upload .env variables to GitHub repo variables
upload-secrets:
	@if [ ! -f .env ]; then echo "Error: .env file not found"; exit 1; fi
	@echo "Uploading .env to GitHub variables for $(GH_REPO)..."
	@$(GH) variable set -f .env --repo $(GH_REPO)
	@echo "✅ Uploaded. Current GitHub variables:"
	@$(GH) variable list --repo $(GH_REPO)

# Download GitHub repo variables to .env
download-secrets:
	@if [ -f .env ]; then \
		echo "⚠️  .env already exists (modified: $$(stat -c '%y' .env 2>/dev/null || stat -f '%Sm' .env 2>/dev/null))"; \
		printf "Overwrite? [y/N]: "; \
		read ans; \
		if [ "$$ans" != "y" ] && [ "$$ans" != "Y" ]; then \
			echo "Aborted."; \
			exit 1; \
		fi; \
	fi
	@echo "Downloading GitHub variables for $(GH_REPO) to .env..."
	@rm -f .env.tmp
	@for var in $(ENV_VARS); do \
		val=$$($(GH) variable get $$var --repo $(GH_REPO) 2>/dev/null); \
		if [ $$? -eq 0 ]; then \
			echo "$$var=$$val" >> .env.tmp; \
		else \
			echo "⚠️  Variable $$var not found on GitHub"; \
		fi; \
	done
	@mv .env.tmp .env
	@echo "✅ Downloaded to .env"


# Security targets (grouped at bottom)

# Run Go vulnerability analysis for reachable vulns.
security-govulncheck:
	@echo "🔎 Running govulncheck $(GOVULNCHECK_VERSION) with Go $(REQUIRED_GO_VERSION)..."
	@$(GOVULNCHECK_CMD) ./...

# Emit govulncheck report as JSON artifact under security_issues/.
security-govulncheck-json:
	mkdir -p security_issues
	$(GOVULNCHECK_CMD) -json ./... > security_issues/govulncheck-$(shell date +%d-%m-%Y).json

# Run OSV scanner against this repository.
security-osv:
	@command -v osv-scanner >/dev/null 2>&1 || { \
		echo "❌ osv-scanner not found. Install from https://github.com/google/osv-scanner"; \
		exit 1; \
	}
	@mkdir -p security_issues
	@osv-scanner --format json . > security_issues/osv-scanner-latest.json
	@echo "✅ Wrote security_issues/osv-scanner-latest.json"

# Regenerate machine-readable and markdown triage artifacts from the latest OSV report.
security-triage: security-osv
	@python3 scripts/extract_osv_report.py \
		--input security_issues/osv-scanner-latest.json \
		--csv security_issues/osv-triage-latest.csv \
		--md security_issues/osv-triage-latest.md
	@echo "✅ Wrote security_issues/osv-triage-latest.csv"
	@echo "✅ Wrote security_issues/osv-triage-latest.md"

# Run gitleaks and emit a dated CSV artifact under security_issues/.
security-gitleaks:
	@command -v gitleaks >/dev/null 2>&1 || { \
		echo "❌ gitleaks not found. Install from https://github.com/gitleaks/gitleaks"; \
		exit 1; \
	}
	@mkdir -p security_issues
	@gitleaks git . -f csv -r security_issues/gitleaks-$(shell date +%d-%m-%Y).csv
	@echo "✅ Wrote security_issues/gitleaks-$(shell date +%d-%m-%Y).csv"

# Audit all B2 object versions under lrc/ using B2 APIs.
security-b2-audit:
	@mkdir -p security_issues
	@/bin/python scripts/b2_release_audit.py \
		--prefix lrc/ \
		--output security_issues/b2-release-audit-$(shell date +%d-%m-%Y).json

# Generate a dry-run deletion plan for unnecessary B2 object versions under lrc/.
security-b2-cleanup-plan:
	@mkdir -p security_issues
	@/bin/python scripts/b2_release_cleanup.py \
		--prefix lrc/ \
		--output security_issues/b2-release-cleanup-plan-$(shell date +%d-%m-%Y).json

# Apply B2 version cleanup plan (destructive). Requires B2 key with deleteFiles capability.
security-b2-cleanup-apply:
	@mkdir -p security_issues
	@/bin/python scripts/b2_release_cleanup.py \
		--prefix lrc/ \
		--output security_issues/b2-release-cleanup-apply-$(shell date +%d-%m-%Y).json \
		--apply

# Backfill or refresh public release manifest from existing B2 release objects.
security-publish-release-manifest:
	@/bin/python scripts/publish_release_manifest.py

# Fail if known leaked B2 literals reappear in tracked source/docs/scripts.
security-secret-regression:
	@! rg -n --hidden --glob '!.git/**' --glob '!security_issues/**' --glob '!Makefile' \
		'K005DV\+hNk6/fdQr8oXHmRsdo8U2YAU|REDACTED_B2_KEY_ID' \
		. >/tmp/lrc-secret-regression.txt || { \
		echo "❌ Secret regression detected:"; \
		cat /tmp/lrc-secret-regression.txt; \
		rm -f /tmp/lrc-secret-regression.txt; \
		exit 1; \
	}
	@rm -f /tmp/lrc-secret-regression.txt
	@echo "✅ No known leaked B2 literals detected in tracked source/docs/scripts"

# Generate SBOMs in both CycloneDX and SPDX formats from Go dependencies.
security-sbom: security-sbom-cyclonedx security-sbom-spdx security-sbom-validate

security-sbom-cyclonedx:
	@command -v $(SYFT_CMD) >/dev/null 2>&1 || { \
		echo "❌ syft not found. Install from https://github.com/anchore/syft"; \
		exit 1; \
	}
	@mkdir -p $(SBOM_DIR)
	@$(SYFT_CMD) file:go.mod --source-name git-lrc --source-version $(SBOM_VERSION) -o cyclonedx-json=$(SBOM_CDX)
	@echo "ℹ️  SBOM version: $(SBOM_VERSION)"
	@echo "✅ Wrote $(SBOM_CDX)"

security-sbom-spdx:
	@command -v $(SYFT_CMD) >/dev/null 2>&1 || { \
		echo "❌ syft not found. Install from https://github.com/anchore/syft"; \
		exit 1; \
	}
	@mkdir -p $(SBOM_DIR)
	@$(SYFT_CMD) file:go.mod --source-name git-lrc --source-version $(SBOM_VERSION) -o spdx-json=$(SBOM_SPDX)
	@echo "ℹ️  SBOM version: $(SBOM_VERSION)"
	@echo "✅ Wrote $(SBOM_SPDX)"

security-sbom-validate:
	@test -s $(SBOM_CDX)
	@test -s $(SBOM_SPDX)
	@echo "✅ SBOM validation passed"

# Generate release notes file and per-release image folder from template.
# Usage: make release-notes-init VERSION=v1.2.3
release-notes-init:
	@if [ -z "$(VERSION)" ]; then \
		echo "❌ VERSION is required. Example: make release-notes-init VERSION=v1.2.3"; \
		exit 1; \
	fi
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { \
		echo "❌ VERSION must match vX.Y.Z"; \
		exit 1; \
	}
	@test -f $(RELEASE_NOTES_TEMPLATE) || { \
		echo "❌ Missing template: $(RELEASE_NOTES_TEMPLATE)"; \
		exit 1; \
	}
	@mkdir -p $(RELEASE_NOTES_DIR) $(RELEASE_IMAGE_DIR)
	@target="$(RELEASE_NOTES_DIR)/$(VERSION).md"; \
	img_dir="$(RELEASE_IMAGE_DIR)/$(VERSION)"; \
	guide="$$img_dir/$(RELEASE_IMAGE_GUIDE)"; \
	if [ -f "$$target" ]; then \
		echo "❌ Release notes already exist: $$target"; \
		exit 1; \
	fi; \
	if [ -e "$$img_dir" ]; then \
		echo "❌ Release image directory already exists: $$img_dir"; \
		exit 1; \
	fi; \
	mkdir -p "$$img_dir"; \
	printf '%s\n' \
		'# Release media for $(VERSION)' \
		'' \
		'Drop screenshots, GIFs, and any local video files for this release into this directory.' \
		'' \
		'Use markdown references like: ![demo](IMG:demo.png)' \
		'For a manual video follow-up, keep this inside an HTML comment in the release notes: <!-- VIDEO:demo.mp4 -->' \
		'' \
		'Raw URL base after publish:' \
		'https://raw.githubusercontent.com/$(GH_REPO)/refs/heads/$(RELEASE_NOTES_BRANCH)/$(RELEASE_IMAGE_DIR)/$(VERSION)/' \
		> "$$guide"; \
	sed \
		-e "s|__VERSION__|$(VERSION)|g" \
		-e "s|__DATE__|$(shell date -u +%Y-%m-%d)|g" \
		-e "s|__IMAGE_DIR__|$(RELEASE_IMAGE_DIR)/$(VERSION)|g" \
		-e "s|__IMAGE_RAW_URL_BASE__|https://raw.githubusercontent.com/$(GH_REPO)/refs/heads/$(RELEASE_NOTES_BRANCH)/$(RELEASE_IMAGE_DIR)/$(VERSION)/|g" \
		-e "s|__IMAGE_RAW_URL_EXAMPLE__|https://raw.githubusercontent.com/$(GH_REPO)/refs/heads/$(RELEASE_NOTES_BRANCH)/$(RELEASE_IMAGE_DIR)/$(VERSION)/demo.png|g" \
		"$(RELEASE_NOTES_TEMPLATE)" > "$$target"; \
	echo "✅ Created $$target"; \
	echo "✅ Created $$img_dir"; \
	echo "✅ Created $$guide"

# Validate release notes file exists and required headings are present.
# Usage: make release-notes-check VERSION=v1.2.3
release-notes-check:
	@if [ -z "$(VERSION)" ]; then \
		echo "❌ VERSION is required. Example: make release-notes-check VERSION=v1.2.3"; \
		exit 1; \
	fi
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { \
		echo "❌ VERSION must match vX.Y.Z"; \
		exit 1; \
	}
	@target="$(RELEASE_NOTES_DIR)/$(VERSION).md"; \
	test -f "$$target" || { echo "❌ Missing release notes: $$target"; exit 1; }; \
	test -s "$$target" || { echo "❌ Release notes file is empty: $$target"; exit 1; }; \
	grep -q '^## Summary' "$$target" || { echo "❌ Missing required section: ## Summary"; exit 1; }; \
	grep -q '^## Install and Update' "$$target" || { echo "❌ Missing required section: ## Install and Update"; exit 1; }; \
	grep -q '^## Changes' "$$target" || { echo "❌ Missing required section: ## Changes"; exit 1; }; \
	python3 $(RELEASE_GH_SCRIPT) --repo $(GH_REPO) --version $(VERSION) --check-only || exit $$?; \
	echo "✅ Release notes validated: $$target"

# Run all release checks before creating/publishing a GitHub release.
# Usage: make release-preflight VERSION=v1.2.3
release-preflight: release-notes-check check-status-doc
	@echo "✅ Release preflight passed for $(VERSION)"

check-status-doc:
	@bash scripts/check-status-doc-links.sh