# Placeholder for the relay server build. Package A1 adds the relay targets.
# Nothing in the game build calls add_subdirectory() on this folder, so the relay is not built with the game.
cmake_minimum_required(VERSION 3.28)

project(servers_relay LANGUAGES CXX)
