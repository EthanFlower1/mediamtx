#include "FrameClock.h"

#include <algorithm>
#include <array>
#include <cassert>

namespace Kaivue::Render {

FrameClock::FrameClock(double budgetMs, QObject* parent)
    : QObject(parent)
    , m_budgetMs(budgetMs)
{}

void FrameClock::beginFrame() noexcept
{
    m_measuring  = true;
    m_frameStart = Clock::now();
}

void FrameClock::endFrame() noexcept
{
    if (!m_measuring) return;
    m_measuring = false;

    const auto now      = Clock::now();
    const double elapsed = std::chrono::duration<double, std::milli>(now - m_frameStart).count();
    m_lastMs = elapsed;

    // Insert into ring buffer
    m_window[m_head] = elapsed;
    m_head = (m_head + 1) % kWindowSize;
    if (m_count < kWindowSize) ++m_count;

    // Only evaluate P99 once we have a meaningful sample size
    if (m_count < 100) return;

    const double p = p99Ms();
    if (p > m_budgetMs) {
        emit frameTimeExceeded(p);
    }
}

double FrameClock::p99Ms() const noexcept
{
    if (m_count == 0) return 0.0;

    // Copy the live portion of the ring buffer into a temporary array for sorting.
    std::array<double, kWindowSize> tmp{};
    for (std::size_t i = 0; i < m_count; ++i) {
        tmp[i] = m_window[i];
    }

    // Partial sort to find the 99th percentile.
    const std::size_t p99Index = static_cast<std::size_t>(
        std::ceil(0.99 * static_cast<double>(m_count))) - 1;

    std::nth_element(tmp.begin(), tmp.begin() + static_cast<std::ptrdiff_t>(p99Index),
                     tmp.begin() + static_cast<std::ptrdiff_t>(m_count));

    return tmp[p99Index];
}

std::size_t FrameClock::frameCount() const noexcept
{
    return m_count;
}

double FrameClock::lastFrameMs() const noexcept
{
    return m_lastMs;
}

} // namespace Kaivue::Render
