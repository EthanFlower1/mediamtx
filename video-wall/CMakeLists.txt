cmake_minimum_required(VERSION 3.24)

#
# Kaivue Video Wall — Qt 6 / C++ desktop client
#
# Wave 1: project scaffold only. No streaming pipeline yet.
# Wave 2+: WebRTC, HLS, RTSP, multi-monitor layout, PTZ hardware.
#
# Build system: CMake + vcpkg manifest mode.
# Targets:      Windows (MSVC 2022), Linux (Ubuntu 22.04 + GCC 12).
# macOS:        deferred.
#

project(KaivueVideoWall
    VERSION 0.1.0
    DESCRIPTION "Kaivue Recording Server — SOC video wall client"
    LANGUAGES CXX
)

# ---------------------------------------------------------------------------
# C++ standard
# ---------------------------------------------------------------------------
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Generate compile_commands.json for clangd / IDE tooling.
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# ---------------------------------------------------------------------------
# Qt automoc / autorcc / autouic
# ---------------------------------------------------------------------------
set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTORCC ON)
set(CMAKE_AUTOUIC ON)

# ---------------------------------------------------------------------------
# Compiler-specific flags
# ---------------------------------------------------------------------------
if(MSVC)
    add_compile_options(/W4 /permissive- /utf-8 /Zc:__cplusplus)
    # Disable a few overly noisy warnings until the codebase grows.
    add_compile_options(/wd4100 /wd4244)
else()
    add_compile_options(-Wall -Wextra -Wpedantic)
endif()

# ---------------------------------------------------------------------------
# Qt 6
# ---------------------------------------------------------------------------
find_package(Qt6 6.6 REQUIRED COMPONENTS
    Core
    Gui
    Quick
    QuickControls2
    Multimedia
    Network
)

qt_standard_project_setup(REQUIRES 6.6)

# ---------------------------------------------------------------------------
# Generated version header
# ---------------------------------------------------------------------------
configure_file(
    "${CMAKE_CURRENT_SOURCE_DIR}/src/version.h.in"
    "${CMAKE_CURRENT_BINARY_DIR}/generated/version.h"
    @ONLY
)

# ---------------------------------------------------------------------------
# Application target
# ---------------------------------------------------------------------------
qt_add_executable(KaivueVideoWall
    src/main.cpp

    # Render core — platform-abstract layer (KAI-333a)
    src/render/TileGrid.cpp
    src/render/QualityController.cpp
    src/render/FrameClock.cpp
    src/render/NullDecoderBackend.cpp

    # Platform backends (guarded by platform preprocessor macros)
    src/render/D3D12Backend.cpp    # Q_OS_WIN only
    src/render/VulkanBackend.cpp   # Q_OS_LINUX only

    # Map view — floor plans + geographic + camera placement (KAI-338)
    src/map/CameraPlacementModel.cpp
    src/map/FilesystemTileProvider.cpp
    src/map/HeatmapOverlay.cpp
    src/map/MapView.cpp

    # Collaboration — multi-user viewing with independent cursors (KAI-339)
    src/collab/CollaborationManager.cpp
    src/collab/ConflictResolver.cpp
    src/collab/CursorOverlay.cpp
)

target_include_directories(KaivueVideoWall PRIVATE
    "${CMAKE_CURRENT_BINARY_DIR}/generated"
    src
)

# QML module — keeps QML side colocated with the executable target.
qt_add_qml_module(KaivueVideoWall
    URI Kaivue.VideoWall
    VERSION 1.0
    QML_FILES
        qml/Main.qml
    RESOURCE_PREFIX /qt/qml
)

target_link_libraries(KaivueVideoWall PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::GuiPrivate   # QRhi lives in private headers (rhi/qrhi.h)
    Qt6::Quick
    Qt6::QuickControls2
    Qt6::Multimedia
    Qt6::Network
)

# Windows: GUI subsystem (no console window).
if(WIN32)
    set_target_properties(KaivueVideoWall PROPERTIES
        WIN32_EXECUTABLE TRUE
    )
endif()

# ---------------------------------------------------------------------------
# Install rules — stubbed for Qt Installer Framework integration in Wave 3.
# EV code signing of the Windows installer is deferred to end-of-roadmap.
# ---------------------------------------------------------------------------
include(GNUInstallDirs)

install(TARGETS KaivueVideoWall
    RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
    BUNDLE  DESTINATION .
    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
)

# Qt deployment helper — populates platform plugins, QML modules, etc.
# Actual installer (Qt IFW on Windows, .deb/.rpm on Linux) is wired up later.
qt_generate_deploy_qml_app_script(
    TARGET KaivueVideoWall
    OUTPUT_SCRIPT deploy_script
    NO_UNSUPPORTED_PLATFORM_ERROR
)
install(SCRIPT ${deploy_script})

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------
enable_testing()
add_subdirectory(tests)
