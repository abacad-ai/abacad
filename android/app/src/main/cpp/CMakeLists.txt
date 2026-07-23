# Native build for the in-app RFB (VNC) server.
#
# We compile LibVNCServer (vendored under libvncserver/) straight into our own
# JNI lib, so live view needs no droidVNC-NG companion. LibVNCServer has its own
# CMake; we drive it via add_subdirectory with every optional feature we don't
# need switched OFF, keeping the build to core RFB + zlib (for ZRLE) and the
# bundled ("included") crypto for VNC auth — no external jpeg/png/ssl/gnutls.
cmake_minimum_required(VERSION 3.22.1)
project(abacad_rfb C)

# --- LibVNCServer feature switches (set before add_subdirectory) -------------
# Off: everything that pulls a system dep or builds examples/tests/the client.
set(WITH_EXAMPLES              OFF CACHE BOOL "" FORCE)
set(WITH_TESTS                 OFF CACHE BOOL "" FORCE)
set(WITH_QT                    OFF CACHE BOOL "" FORCE)
set(WITH_SDL                   OFF CACHE BOOL "" FORCE)
set(WITH_GTK                   OFF CACHE BOOL "" FORCE)
set(WITH_GNUTLS                OFF CACHE BOOL "" FORCE)
set(WITH_OPENSSL               OFF CACHE BOOL "" FORCE)
set(WITH_GCRYPT                OFF CACHE BOOL "" FORCE)
set(WITH_FFMPEG                OFF CACHE BOOL "" FORCE)
set(WITH_LIBSSHTUNNEL          OFF CACHE BOOL "" FORCE)
set(WITH_SYSTEMD               OFF CACHE BOOL "" FORCE)
set(WITH_SASL                  OFF CACHE BOOL "" FORCE)
set(WITH_XCB                   OFF CACHE BOOL "" FORCE)
set(WITH_WEBSOCKETS            OFF CACHE BOOL "" FORCE)  # our transport is raw RFB over localhost; the server bridges WS<->TCP
set(WITH_TIGHTVNC_FILETRANSFER OFF CACHE BOOL "" FORCE)
set(WITH_LIBVNCCLIENT          OFF CACHE BOOL "" FORCE)  # server only
set(LIBVNCSERVER_INSTALL       OFF CACHE BOOL "" FORCE)
# On: zlib (ZRLE encoding) + threads (background server loop). JPEG/PNG are auto-
# skipped when not found, disabling Tight for now — a follow-up can add
# libjpeg-turbo for it.
set(WITH_ZLIB                  ON  CACHE BOOL "" FORCE)
set(WITH_THREADS               ON  CACHE BOOL "" FORCE)
# Static: fold libvncserver into our single .so.
set(BUILD_SHARED_LIBS          OFF CACHE BOOL "" FORCE)

add_subdirectory(libvncserver)

# Silence warnings from the vendored upstream: we pin LibVNCServer as a submodule
# and don't patch its source, so its -Wswitch / tautological-compare / zlib
# const-cast / open()-mode warnings are just noise in our build. Scoped to the
# vncserver target only — our own rfb_jni.c keeps full warnings.
target_compile_options(vncserver PRIVATE -w)

# --- our JNI lib -------------------------------------------------------------
add_library(abacad_rfb SHARED
    rfb_jni.c
)

target_include_directories(abacad_rfb PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/libvncserver/include            # rfb/rfb.h
    ${CMAKE_CURRENT_BINARY_DIR}/libvncserver/include            # generated rfb/rfbconfig.h
)

find_library(log-lib log)
target_link_libraries(abacad_rfb vncserver ${log-lib})
