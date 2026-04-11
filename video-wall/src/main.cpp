// Kaivue Video Wall — entry point.
//
// Wave 1: minimum-viable Qt Quick application that opens an empty window.
// Wave 2+: stream pipeline, multi-monitor layout, PTZ hardware integration.

#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QLoggingCategory>
#include <QVersionNumber>

#include "crash/CrashReporter.h"
#include "updater/UpdateChecker.h"
#include "version.h"

int main(int argc, char *argv[])
{
    QGuiApplication::setOrganizationName(QStringLiteral("Kaivue"));
    QGuiApplication::setOrganizationDomain(QStringLiteral("kaivue.com"));
    QGuiApplication::setApplicationName(QStringLiteral("Kaivue Video Wall"));
    QGuiApplication::setApplicationVersion(
        QStringLiteral(KAIVUE_VIDEOWALL_VERSION));

    // Install the crash reporter facade as early as possible so any later
    // startup failure is captured. Wave 1 is a no-op stub; the Sentry /
    // Crashpad backend swaps in during Wave 6 hardening.
    {
        kaivue::crash::Config cfg;
        cfg.release = QStringLiteral(KAIVUE_VIDEOWALL_VERSION);
        cfg.environment = QStringLiteral("dev");
        cfg.disabled = true; // Wave 1: no upload, no minidump writer.
        kaivue::crash::install(cfg);
    }

    // Use the Fusion-derived "Basic" style by default; SOC operators get a
    // custom theme later in Wave 2.
    QQuickStyle::setStyle(QStringLiteral("Basic"));

    QGuiApplication app(argc, argv);

    QLoggingCategory::setFilterRules(QStringLiteral("qt.qml.debug=false"));

    QQmlApplicationEngine engine;

    QObject::connect(
        &engine,
        &QQmlApplicationEngine::objectCreationFailed,
        &app,
        []() { QCoreApplication::exit(EXIT_FAILURE); },
        Qt::QueuedConnection);

    // ── In-app updater (KAI-340) ──────────────────────────────────────────
    // Polls the update manifest once per hour. On Windows, applies updates
    // via the Qt IFW MaintenanceTool; on Linux, directs the user to the
    // package repository.
    kaivue::updater::UpdateChecker updater;
    updater.setCurrentVersion(
        QVersionNumber::fromString(
            QStringLiteral(KAIVUE_VIDEOWALL_VERSION)));
    updater.setManifestUrl(
        QStringLiteral("https://updates.kaivue.com/videowall"));
    updater.setCheckIntervalSecs(3600);
    updater.start();

    // Expose the updater to QML so the UI can bind to update notifications.
    engine.rootContext()->setContextProperty(
        QStringLiteral("updateChecker"), &updater);

    engine.loadFromModule(QStringLiteral("Kaivue.VideoWall"),
                          QStringLiteral("Main"));

    return app.exec();
}
