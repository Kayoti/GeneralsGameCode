# Build file for the Servers relay. It is standalone: the game build never calls add_subdirectory() on this folder.
#
# Phase 1 supports Windows (Win32 and x64) with the Visual Studio toolset only.
# Linux is package 4.1.
#
# Configure and build:
#   cmake -S Servers/Relay -B <build dir> -G Ninja -DCMAKE_BUILD_TYPE=Release
#   cmake --build <build dir>
# Tests (optional):
#   cmake -S Servers/Relay -B <build dir> -DRELAY_BUILD_TESTS=ON
#   cmake --build <build dir> --target relay_tests
#   ctest --test-dir <build dir> -C Release

cmake_minimum_required(VERSION 3.28)

project(servers_relay LANGUAGES C CXX)

if(NOT MSVC)
    message(FATAL_ERROR "The Servers relay supports the MSVC toolset only in Phase 1.")
endif()

option(RELAY_BUILD_TESTS "Build the relay unit tests" OFF)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# The relay links libsodium's static library, so it uses the static runtime to match.
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")

include(FetchContent)

# ENet 1.3.18. Its CMake file asks for a minimum CMake version below 3.5, which CMake 4 no longer accepts.
set(CMAKE_POLICY_VERSION_MINIMUM 3.5)
FetchContent_Declare(
    enet
    GIT_REPOSITORY https://github.com/lsalzman/enet.git
    GIT_TAG        2662c0de09e36f2a2030ccc2c528a3e4c9e8138a # v1.3.18
)
FetchContent_MakeAvailable(enet)
unset(CMAKE_POLICY_VERSION_MINIMUM)
# ENet's own CMake file does not export its public headers to consumers.
target_include_directories(enet PUBLIC ${enet_SOURCE_DIR}/include)

# libsodium 1.0.22. The release has no CMake build, so the prebuilt MSVC static library from the
# official release is used. The archive is checked against the SHA-256 digest published on the release.
if(CMAKE_SIZEOF_VOID_P EQUAL 8)
    set(RELAY_SODIUM_ARCH x64)
else()
    set(RELAY_SODIUM_ARCH Win32)
endif()

set(RELAY_SODIUM_ZIP ${CMAKE_BINARY_DIR}/_deps/libsodium-1.0.22-msvc.zip)
set(RELAY_SODIUM_DIR ${CMAKE_BINARY_DIR}/_deps/libsodium-1.0.22-msvc)

file(DOWNLOAD
    https://github.com/jedisct1/libsodium/releases/download/1.0.22-RELEASE/libsodium-1.0.22-msvc.zip
    ${RELAY_SODIUM_ZIP}
    EXPECTED_HASH SHA256=3e03a726fac4bc09cb61d8f29d658ef7a5eca0811de59082130414f7ca2e4279
    TLS_VERIFY ON
)
if(NOT EXISTS ${RELAY_SODIUM_DIR}/libsodium/include/sodium.h)
    file(ARCHIVE_EXTRACT INPUT ${RELAY_SODIUM_ZIP} DESTINATION ${RELAY_SODIUM_DIR})
endif()

add_library(libsodium STATIC IMPORTED GLOBAL)
set_target_properties(libsodium PROPERTIES
    IMPORTED_CONFIGURATIONS "Debug;Release"
    IMPORTED_LOCATION_DEBUG   ${RELAY_SODIUM_DIR}/libsodium/${RELAY_SODIUM_ARCH}/Debug/v143/static/libsodium.lib
    IMPORTED_LOCATION_RELEASE ${RELAY_SODIUM_DIR}/libsodium/${RELAY_SODIUM_ARCH}/Release/v143/static/libsodium.lib
    MAP_IMPORTED_CONFIG_MINSIZEREL Release
    MAP_IMPORTED_CONFIG_RELWITHDEBINFO Release
    INTERFACE_INCLUDE_DIRECTORIES ${RELAY_SODIUM_DIR}/libsodium/include
    INTERFACE_COMPILE_DEFINITIONS SODIUM_STATIC
)

# Shared relay code. The game engine is not used.
add_library(relay_core STATIC
    src/config.cpp
    src/config.h
    src/drop.cpp
    src/entry_check.cpp
    src/entry_check.h
    src/host.cpp
    src/host.h
    src/log.h
    src/match_data.cpp
    src/pass.cpp
    src/pass.h
    src/protocol.h
    src/rate_limit.cpp
    src/rate_limit.h
    src/records.cpp
    src/records.h
    src/region_keys.cpp
    src/region_keys.h
    src/sessions.cpp
    src/sessions.h
    src/text.h
    src/wire.h
)

target_include_directories(relay_core PUBLIC src)
target_link_libraries(relay_core PUBLIC enet libsodium)
target_compile_definitions(relay_core PRIVATE _CRT_SECURE_NO_WARNINGS)
target_compile_options(relay_core PRIVATE /W4 /WX)

add_executable(relay src/main.cpp)
target_link_libraries(relay PRIVATE relay_core ws2_32 winmm)
target_compile_options(relay PRIVATE /W4 /WX)

# Command-line entry check client for hand tests (docs/servers/packages/A2-relay-gate.md).
add_executable(entry_check_client tools/entry_check_client.cpp)
target_link_libraries(entry_check_client PRIVATE relay_core ws2_32 winmm)
target_compile_options(entry_check_client PRIVATE /W4 /WX)

# Test client for sessions: creates or joins a session and prints what the relay sends.
add_executable(session_client tools/session_client.cpp)
target_link_libraries(session_client PRIVATE relay_core ws2_32 winmm)
target_compile_definitions(session_client PRIVATE _CRT_SECURE_NO_WARNINGS)
target_compile_options(session_client PRIVATE /W4 /WX)

if(RELAY_BUILD_TESTS)
    enable_testing()
    add_executable(relay_tests tests/relay_tests.cpp)
    target_link_libraries(relay_tests PRIVATE relay_core)
    target_compile_definitions(relay_tests PRIVATE _CRT_SECURE_NO_WARNINGS)
    target_compile_options(relay_tests PRIVATE /W4 /WX)
    add_test(NAME relay_tests COMMAND relay_tests)
endif()
