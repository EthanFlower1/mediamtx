cmake_minimum_required(VERSION 3.24)

# ---------------------------------------------------------------------------
# Render core unit tests
# Requires Qt 6.6+ with Qt6::Test and Qt6::GuiPrivate
# ---------------------------------------------------------------------------

find_package(Qt6 6.6 REQUIRED COMPONENTS Test Gui)

# tst_render_core — uses QOffscreenSurface + NullDecoderBackend (no GPU)
qt_add_executable(tst_render_core
    render/tst_render_core.cpp

    # Render core sources (compiled again in the test target to avoid
    # the need for a separate shared library at this scaffold stage)
    "${CMAKE_SOURCE_DIR}/src/render/TileGrid.cpp"
    "${CMAKE_SOURCE_DIR}/src/render/QualityController.cpp"
    "${CMAKE_SOURCE_DIR}/src/render/FrameClock.cpp"
    "${CMAKE_SOURCE_DIR}/src/render/NullDecoderBackend.cpp"
)

target_include_directories(tst_render_core PRIVATE
    "${CMAKE_SOURCE_DIR}/src"
)

target_link_libraries(tst_render_core PRIVATE
    Qt6::Test
    Qt6::Gui
    Qt6::GuiPrivate  # QRhi / rhi/qrhi.h lives in GuiPrivate
    Qt6::Core
)

# Enable test discovery
add_test(NAME tst_render_core COMMAND tst_render_core)
set_tests_properties(tst_render_core PROPERTIES
    LABELS "render"
    TIMEOUT 120
)

# ---------------------------------------------------------------------------
# Map view unit tests (KAI-338)
# Headless — uses NullTileProvider and offscreen QImage painting; no GPU,
# no network, no real map tiles. Safe for CI containers.
# ---------------------------------------------------------------------------
qt_add_executable(tst_map_view
    map/tst_map_view.cpp

    # Map sources (compiled into the test target so no shared library
    # is needed at this scaffold stage — mirrors tst_render_core).
    "${CMAKE_SOURCE_DIR}/src/map/CameraPlacementModel.cpp"
    "${CMAKE_SOURCE_DIR}/src/map/FilesystemTileProvider.cpp"
    "${CMAKE_SOURCE_DIR}/src/map/HeatmapOverlay.cpp"
    "${CMAKE_SOURCE_DIR}/src/map/MapView.cpp"
)

target_include_directories(tst_map_view PRIVATE
    "${CMAKE_SOURCE_DIR}/src"
)

target_link_libraries(tst_map_view PRIVATE
    Qt6::Test
    Qt6::Gui
    Qt6::Core
)

add_test(NAME tst_map_view COMMAND tst_map_view)
set_tests_properties(tst_map_view PROPERTIES
    LABELS "map"
    TIMEOUT 120
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen"
)

# ---------------------------------------------------------------------------
# Update checker unit tests (KAI-340)
# Headless — validates manifest parsing, version comparison, and signal
# wiring without network access. Safe for CI containers.
# ---------------------------------------------------------------------------
qt_add_executable(tst_update_checker
    updater/tst_update_checker.cpp

    "${CMAKE_SOURCE_DIR}/src/updater/UpdateChecker.cpp"
)

target_include_directories(tst_update_checker PRIVATE
    "${CMAKE_SOURCE_DIR}/src"
)

target_link_libraries(tst_update_checker PRIVATE
    Qt6::Test
    Qt6::Core
    Qt6::Network
)

add_test(NAME tst_update_checker COMMAND tst_update_checker)
set_tests_properties(tst_update_checker PROPERTIES
    LABELS "updater"
    TIMEOUT 60
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen"
)
