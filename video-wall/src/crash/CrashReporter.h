// Kaivue Video Wall — crash reporter facade.
//
// Wave 1: stub. Provides a stable API surface so the rest of the codebase can
// call install() / setUserContext() / addBreadcrumb() without caring whether
// a real backend is wired up.
//
// Wave 2+: switch the implementation between Sentry Native SDK
// (sentry-native) on Linux and Crashpad on Windows. Backend selection is a
// build-time decision driven by vcpkg features; the public API stays the
// same so call sites do not move.
//
// See KAI-332 (scaffold) and the future KAI- ticket that lands real crash
// uploads as part of Wave 6 hardening.

#pragma once

#include <QString>

namespace kaivue::crash {

struct Config {
    // DSN / upload URL. Empty in Wave 1 — disables upload.
    QString dsn;
    // Release identifier; defaults to KAIVUE_VIDEOWALL_VERSION at install().
    QString release;
    // "production" / "staging" / "dev". Defaults to "dev".
    QString environment;
    // If true the reporter is fully disabled (no minidump writer, no hooks).
    bool disabled = true;
};

// Installs the crash handler. Safe to call exactly once at startup, before
// any QGuiApplication interaction. In Wave 1 this is a no-op that just logs
// the configuration to qDebug() so we can prove the call site exists.
void install(const Config &config);

// Records a structured breadcrumb. No-op in Wave 1.
void addBreadcrumb(const QString &category, const QString &message);

// Associates the current operator with subsequent crash reports.
// No-op in Wave 1.
void setUser(const QString &id, const QString &email);

} // namespace kaivue::crash
