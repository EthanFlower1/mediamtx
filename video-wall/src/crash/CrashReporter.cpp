// Kaivue Video Wall — crash reporter facade (Wave 1 stub).
//
// Real Sentry Native / Crashpad backend lands in Wave 6 (KAI-340 / hardening).
// The stub keeps the public API stable so call sites do not move when the
// backend swaps in.

#include "CrashReporter.h"

#include <QLoggingCategory>

Q_LOGGING_CATEGORY(lcCrash, "kaivue.crash")

namespace kaivue::crash {

namespace {
bool g_installed = false;
} // namespace

void install(const Config &config)
{
    if (g_installed) {
        qCWarning(lcCrash) << "install() called twice — ignoring";
        return;
    }
    g_installed = true;

    qCInfo(lcCrash).nospace()
        << "crash reporter stub installed (disabled=" << config.disabled
        << ", environment=" << (config.environment.isEmpty()
                                    ? QStringLiteral("dev")
                                    : config.environment)
        << ", release=" << config.release
        << ", dsn_set=" << !config.dsn.isEmpty() << ")";

    // Wave 2+: branch on Q_OS_WIN -> Crashpad, Q_OS_LINUX -> sentry-native.
    // For Wave 1 there is no minidump writer, no symbol upload, and no
    // network traffic.
}

void addBreadcrumb(const QString &category, const QString &message)
{
    if (!g_installed) {
        return;
    }
    qCDebug(lcCrash).nospace()
        << "breadcrumb [" << category << "] " << message;
}

void setUser(const QString &id, const QString &email)
{
    if (!g_installed) {
        return;
    }
    qCDebug(lcCrash).nospace()
        << "setUser id=" << id << " email=" << email;
}

} // namespace kaivue::crash
