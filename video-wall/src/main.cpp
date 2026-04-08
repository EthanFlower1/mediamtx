// Kaivue Video Wall — entry point.
//
// Wave 1: minimum-viable Qt Quick application that opens an empty window.
// Wave 2+: stream pipeline, multi-monitor layout, PTZ hardware integration.

#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQuickStyle>
#include <QLoggingCategory>

#include "version.h"

int main(int argc, char *argv[])
{
    QGuiApplication::setOrganizationName(QStringLiteral("Kaivue"));
    QGuiApplication::setOrganizationDomain(QStringLiteral("kaivue.com"));
    QGuiApplication::setApplicationName(QStringLiteral("Kaivue Video Wall"));
    QGuiApplication::setApplicationVersion(
        QStringLiteral(KAIVUE_VIDEOWALL_VERSION));

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

    engine.loadFromModule(QStringLiteral("Kaivue.VideoWall"),
                          QStringLiteral("Main"));

    return app.exec();
}
