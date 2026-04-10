cmake_minimum_required(VERSION 3.24)

# ---------------------------------------------------------------------------
# Render core unit tests
# Requires Qt 6.6+ with Qt6::Test and Qt6::GuiPrivate
# ---------------------------------------------------------------------------

find_package(Qt6 6.6 REQUIRED COMPONENTS Test Gui Quick)

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
# tst_collab — KAI-339 multi-user viewing with independent cursors
# Headless, CI-safe, LoopbackTransport only (no network).
# ---------------------------------------------------------------------------
qt_add_executable(tst_collab
    collab/tst_collab.cpp

    # Collab sources compiled into the test binary (same pattern as
    # tst_render_core — avoids a separate static library at this stage).
    "${CMAKE_SOURCE_DIR}/src/collab/CollaborationManager.cpp"
    "${CMAKE_SOURCE_DIR}/src/collab/ConflictResolver.cpp"
    "${CMAKE_SOURCE_DIR}/src/collab/CursorOverlay.cpp"
)

target_include_directories(tst_collab PRIVATE
    "${CMAKE_SOURCE_DIR}/src"
)

target_link_libraries(tst_collab PRIVATE
    Qt6::Test
    Qt6::Core
    Qt6::Gui
    Qt6::Quick
)

add_test(NAME tst_collab COMMAND tst_collab)
set_tests_properties(tst_collab PROPERTIES
    LABELS "collab"
    TIMEOUT 60
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen"
)
