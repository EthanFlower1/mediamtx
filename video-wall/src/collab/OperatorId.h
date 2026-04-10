#pragma once

#include <QString>
#include <QtGlobal>

#include <cstdint>
#include <functional>

namespace Kaivue::Collab {

/**
 * Strong-typed operator identifier.
 *
 * Wraps a 64-bit value so OperatorId cannot be implicitly confused with
 * TileId, tenant id, or any other integer-based identifier elsewhere in
 * the Video Wall codebase.
 */
struct OperatorId {
    std::uint64_t value{0};

    constexpr OperatorId() noexcept = default;
    constexpr explicit OperatorId(std::uint64_t v) noexcept : value(v) {}

    [[nodiscard]] constexpr bool isValid() const noexcept { return value != 0; }

    friend constexpr bool operator==(OperatorId a, OperatorId b) noexcept {
        return a.value == b.value;
    }
    friend constexpr bool operator!=(OperatorId a, OperatorId b) noexcept {
        return a.value != b.value;
    }
    friend constexpr bool operator<(OperatorId a, OperatorId b) noexcept {
        return a.value < b.value;
    }
};

} // namespace Kaivue::Collab

namespace std {
template <>
struct hash<Kaivue::Collab::OperatorId> {
    std::size_t operator()(Kaivue::Collab::OperatorId id) const noexcept {
        return std::hash<std::uint64_t>{}(id.value);
    }
};
} // namespace std
