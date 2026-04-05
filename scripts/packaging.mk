PACKAGING_VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")

.PHONY: package-deb package-rpm package-macos package-windows package-all

package-deb: ## Build Debian/Ubuntu .deb package
	@echo "==> Building DEB package v$(PACKAGING_VERSION)..."
	@mkdir -p tmp
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags enable_upgrade -o tmp/mediamtx .
	@mkdir -p tmp/deb-root/DEBIAN
	@mkdir -p tmp/deb-root/usr/bin
	@mkdir -p tmp/deb-root/etc/mediamtx
	@mkdir -p tmp/deb-root/lib/systemd/system
	@mkdir -p tmp/deb-root/var/lib/mediamtx/recordings
	@mkdir -p tmp/deb-root/var/lib/mediamtx/thumbnails
	@mkdir -p tmp/deb-root/var/log/mediamtx
	cp tmp/mediamtx tmp/deb-root/usr/bin/mediamtx
	chmod 0755 tmp/deb-root/usr/bin/mediamtx
	cp mediamtx.yml tmp/deb-root/etc/mediamtx/mediamtx.yml
	chmod 0640 tmp/deb-root/etc/mediamtx/mediamtx.yml
	cp packaging/linux/debian/mediamtx.service tmp/deb-root/lib/systemd/system/mediamtx.service
	@echo "Package: mediamtx" > tmp/deb-root/DEBIAN/control
	@echo "Version: $(PACKAGING_VERSION)" >> tmp/deb-root/DEBIAN/control
	@echo "Section: video" >> tmp/deb-root/DEBIAN/control
	@echo "Priority: optional" >> tmp/deb-root/DEBIAN/control
	@echo "Architecture: amd64" >> tmp/deb-root/DEBIAN/control
	@echo "Depends: adduser" >> tmp/deb-root/DEBIAN/control
	@echo "Maintainer: MediaMTX Maintainers <support@mediamtx.dev>" >> tmp/deb-root/DEBIAN/control
	@echo "Description: Real-time media server and network video recorder" >> tmp/deb-root/DEBIAN/control
	cp packaging/linux/debian/postinst tmp/deb-root/DEBIAN/postinst
	cp packaging/linux/debian/prerm tmp/deb-root/DEBIAN/prerm
	cp packaging/linux/debian/postrm tmp/deb-root/DEBIAN/postrm
	cp packaging/linux/debian/conffiles tmp/deb-root/DEBIAN/conffiles
	chmod 0755 tmp/deb-root/DEBIAN/postinst tmp/deb-root/DEBIAN/prerm tmp/deb-root/DEBIAN/postrm
	@mkdir -p binaries
	dpkg-deb --build --root-owner-group tmp/deb-root "binaries/mediamtx_$(PACKAGING_VERSION)_amd64.deb"
	@rm -rf tmp/deb-root
	@echo "==> DEB package: binaries/mediamtx_$(PACKAGING_VERSION)_amd64.deb"

package-rpm: ## Build RPM package
	@echo "==> Building RPM package v$(PACKAGING_VERSION)..."
	@mkdir -p tmp/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
	git archive --format=tar.gz --prefix=mediamtx-$(PACKAGING_VERSION)/ HEAD > tmp/rpmbuild/SOURCES/mediamtx-$(PACKAGING_VERSION).tar.gz
	rpmbuild -bb \
		--define "_topdir $(CURDIR)/tmp/rpmbuild" \
		--define "_version $(PACKAGING_VERSION)" \
		packaging/linux/rpm/mediamtx.spec
	@mkdir -p binaries
	cp tmp/rpmbuild/RPMS/*/mediamtx-*.rpm binaries/ || true
	@rm -rf tmp/rpmbuild
	@echo "==> RPM package built in binaries/"

package-macos: ## Build macOS .pkg installer
	@echo "==> Building macOS package v$(PACKAGING_VERSION)..."
	@mkdir -p tmp
	CGO_ENABLED=0 go build -tags enable_upgrade -o tmp/mediamtx .
	chmod +x packaging/macos/build-pkg.sh
	packaging/macos/build-pkg.sh "$(PACKAGING_VERSION)"
	@echo "==> macOS package: binaries/mediamtx-nvr-$(PACKAGING_VERSION)-macos.pkg"

package-windows: ## Build Windows NSIS installer (requires NSIS on PATH)
	@echo "==> Building Windows installer v$(PACKAGING_VERSION)..."
	@mkdir -p tmp
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags enable_upgrade -o tmp/mediamtx.exe .
	makensis -DPRODUCT_VERSION="$(PACKAGING_VERSION)" packaging/windows/mediamtx.nsi
	@mkdir -p binaries
	mv packaging/windows/mediamtx-nvr-$(PACKAGING_VERSION)-setup.exe binaries/ || true
	@echo "==> Windows installer: binaries/mediamtx-nvr-$(PACKAGING_VERSION)-setup.exe"

package-all: package-deb package-rpm package-macos package-windows ## Build all platform packages
	@echo "==> All packages built in binaries/"
