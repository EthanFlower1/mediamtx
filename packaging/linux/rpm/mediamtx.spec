%global debug_package %{nil}

Name:           mediamtx
Version:        %{?_version}%{!?_version:0.0.0}
Release:        1%{?dist}
Summary:        Real-time media server and network video recorder
License:        MIT
URL:            https://github.com/bluenviron/mediamtx
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.25
BuildRequires:  nodejs >= 20
BuildRequires:  npm
BuildRequires:  systemd-rpm-macros

Requires:       systemd
Requires(pre):  shadow-utils

%description
MediaMTX NVR is a production-grade media server with built-in NVR
capabilities. It supports RTSP, RTMP, HLS, WebRTC, and SRT protocols
with ONVIF camera auto-discovery, SQLite-backed recording management,
and a web-based administration interface.

%pre
# Create mediamtx system user
getent group mediamtx >/dev/null || groupadd -r mediamtx
getent passwd mediamtx >/dev/null || \
    useradd -r -g mediamtx -d /var/lib/mediamtx -s /sbin/nologin \
    -c "MediaMTX NVR" mediamtx
exit 0

%prep
%autosetup -n %{name}-%{version}

%build
CGO_ENABLED=0 go build -tags enable_upgrade -o %{name} .

%install
# Binary
install -D -m 0755 %{name} %{buildroot}%{_bindir}/%{name}

# Configuration
install -D -m 0640 mediamtx.yml %{buildroot}%{_sysconfdir}/%{name}/mediamtx.yml

# Systemd unit
install -D -m 0644 packaging/linux/debian/mediamtx.service %{buildroot}%{_unitdir}/%{name}.service

# Data directories
install -d -m 0755 %{buildroot}%{_sharedstatedir}/%{name}
install -d -m 0755 %{buildroot}%{_sharedstatedir}/%{name}/recordings
install -d -m 0755 %{buildroot}%{_sharedstatedir}/%{name}/thumbnails

# Log directory
install -d -m 0750 %{buildroot}%{_localstatedir}/log/%{name}

%post
%systemd_post %{name}.service
chown -R mediamtx:mediamtx %{_sharedstatedir}/%{name}
chown -R mediamtx:mediamtx %{_localstatedir}/log/%{name}
chown root:mediamtx %{_sysconfdir}/%{name}/mediamtx.yml

%preun
%systemd_preun %{name}.service

%postun
%systemd_postun_with_restart %{name}.service

if [ $1 -eq 0 ]; then
    # Package removal (not upgrade) - clean up user
    userdel mediamtx 2>/dev/null || true
    groupdel mediamtx 2>/dev/null || true
fi

%files
%license LICENSE
%{_bindir}/%{name}
%dir %{_sysconfdir}/%{name}
%config(noreplace) %{_sysconfdir}/%{name}/mediamtx.yml
%{_unitdir}/%{name}.service
%dir %attr(0755,mediamtx,mediamtx) %{_sharedstatedir}/%{name}
%dir %attr(0755,mediamtx,mediamtx) %{_sharedstatedir}/%{name}/recordings
%dir %attr(0755,mediamtx,mediamtx) %{_sharedstatedir}/%{name}/thumbnails
%dir %attr(0750,mediamtx,mediamtx) %{_localstatedir}/log/%{name}

%changelog
* Thu Apr 03 2026 MediaMTX Maintainers <support@mediamtx.dev> - 0.0.0-1
- Initial RPM packaging
