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
# Hardware decoder backend feature flags (KAI-333)
#
# All OFF by default so CI (NullDecoderBackend) has no ffmpeg/libva/
# VideoToolbox/D3D11VA dependency. Opt in per-platform on developer
# machines or on release build agents.
# ---------------------------------------------------------------------------
option(KAIVUE_ENABLE_VAAPI          "Build VA-API (Linux) HW decoder backend"          OFF)
option(KAIVUE_ENABLE_VIDEOTOOLBOX   "Build VideoToolbox (macOS) HW decoder backend"    OFF)
option(KAIVUE_ENABLE_D3D11          "Build D3D11VA (Windows) HW decoder backend"       OFF)

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

    # Metrics (KAI-333)
    src/render/metrics/P99LatencyProbe.cpp

    # Platform backends (guarded by platform preprocessor macros)
    src/render/D3D12Backend.cpp    # Q_OS_WIN only
    src/render/VulkanBackend.cpp   # Q_OS_LINUX only

    # Map view — floor plans + geographic + camera placement (KAI-338)
    src/map/CameraPlacementModel.cpp
    src/map/FilesystemTileProvider.cpp
    src/map/HeatmapOverlay.cpp
    src/map/MapView.cpp

    # HW decoder backends (KAI-333 — guarded by feature flags below;
    # inert translation units when the corresponding flag is OFF)
    src/render/VaapiDecoderBackend.cpp
    src/render/VideoToolboxDecoderBackend.cpp
    src/render/D3D11DecoderBackend.cpp
)

# ---------------------------------------------------------------------------
# HW decoder backend wiring (KAI-333)
# ---------------------------------------------------------------------------
if(KAIVUE_ENABLE_VAAPI)
    find_package(PkgConfig REQUIRED)
    pkg_check_modules(FFMPEG_VAAPI REQUIRED
        libavcodec libavutil libavformat libva libva-drm)
    target_compile_definitions(KaivueVideoWall PRIVATE KAIVUE_ENABLE_VAAPI)
    target_include_directories(KaivueVideoWall PRIVATE ${FFMPEG_VAAPI_INCLUDE_DIRS})
    target_link_libraries(KaivueVideoWall PRIVATE ${FFMPEG_VAAPI_LIBRARIES})
    message(STATUS "KAI-333: VA-API HW decoder backend ENABLED")
endif()

if(KAIVUE_ENABLE_VIDEOTOOLBOX)
    find_package(PkgConfig REQUIRED)
    pkg_check_modules(FFMPEG_VT REQUIRED libavcodec libavutil libavformat)
    target_compile_definitions(KaivueVideoWall PRIVATE KAIVUE_ENABLE_VIDEOTOOLBOX)
    target_include_directories(KaivueVideoWall PRIVATE ${FFMPEG_VT_INCLUDE_DIRS})
    target_link_libraries(KaivueVideoWall PRIVATE
        ${FFMPEG_VT_LIBRARIES}
        "-framework VideoToolbox"
        "-framework CoreMedia"
        "-framework CoreVideo"
        "-framework CoreFoundation")
    message(STATUS "KAI-333: VideoToolbox HW decoder backend ENABLED")
endif()

if(KAIVUE_ENABLE_D3D11)
    find_package(PkgConfig REQUIRED)
    pkg_check_modules(FFMPEG_D3D11 REQUIRED libavcodec libavutil libavformat)
    target_compile_definitions(KaivueVideoWall PRIVATE KAIVUE_ENABLE_D3D11)
    target_include_directories(KaivueVideoWall PRIVATE ${FFMPEG_D3D11_INCLUDE_DIRS})
    target_link_libraries(KaivueVideoWall PRIVATE
        ${FFMPEG_D3D11_LIBRARIES}
        d3d11 dxgi)
    message(STATUS "KAI-333: D3D11VA HW decoder backend ENABLED")
endif()

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
