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
