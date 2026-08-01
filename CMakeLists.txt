# re2-wasm CMakeLists.txt
#
# Phase 1 has no C/C++ build. This file exists so that future phases can plug
# in a native asset-converter toolchain (BIN/CUE extractors, TIM/TMD decoders
# that benefit from C performance) without re-shaping the repo. Enabling this
# build is opt-in: it only runs when `cmake -DBUILD_NATIVE_TOOLS=ON` is set.

cmake_minimum_required(VERSION 3.18)

project(re2-wasm
    VERSION 0.1.0
    DESCRIPTION "Browser port of Resident Evil 2 (1998) built on OpenBiohazard2"
    LANGUAGES C CXX)

option(BUILD_NATIVE_TOOLS "Build the native C/C++ asset-converter tools" OFF)

if(NOT BUILD_NATIVE_TOOLS)
    message(STATUS "re2-wasm: BUILD_NATIVE_TOOLS is OFF — skipping C/C++ build.")
    message(STATUS "  Enable with: cmake -DBUILD_NATIVE_TOOLS=ON ..")
    # Nothing else to do; the Go Makefile handles everything else.
    return()
endif()

# ---------------------------------------------------------------------------
# Native toolchain (Phase 3+ placeholder)
# ---------------------------------------------------------------------------

# When BUILD_NATIVE_TOOLS is ON we build a small CLI that converts original
# RE2 asset formats into engine-friendly blobs (decoded TIM textures, indexed
# TMD meshes, etc.). The converter is shipped as a separate binary so the WASM
# module stays small; users run the converter once per game dump.

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Treat warnings as errors — matches the Go side's zero-warning policy.
if(MSVC)
    add_compile_options(/W4 /WX)
else()
    add_compile_options(-Wall -Wextra -Wpedantic -Werror)
endif()

add_subdirectory(native/tools EXCLUDE_FROM_ALL)

message(STATUS "re2-wasm: native tools enabled.")
