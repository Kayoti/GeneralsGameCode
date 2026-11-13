# ENet carries the relay link of Servers mode. It is fetched only when the Servers switch is on.
if(NOT RTS_BUILD_OPTION_SERVERS)
    return()
endif()

FetchContent_Declare(
    enet
    GIT_REPOSITORY https://github.com/lsalzman/enet.git
    GIT_TAG        2662c0de09e36f2a2030ccc2c528a3e4c9e8138a # v1.3.18
    EXCLUDE_FROM_ALL
)

FetchContent_MakeAvailable(enet)
