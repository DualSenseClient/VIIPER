package sdl

/*
#cgo CFLAGS: -I${SRCDIR}/../deps/SDL/include

#include <SDL3/SDL_events.h>
#include <SDL3/SDL_mutex.h>

#define AXIS_WATCH_CAPACITY 65536

static SDL_Mutex *axis_watch_mutex;
static SDL_JoystickID axis_watch_gamepad;
static Uint8 axis_watch_axis;
static Sint16 axis_watch_values[AXIS_WATCH_CAPACITY];
static Uint32 axis_watch_head;
static Uint32 axis_watch_count;
static Uint64 axis_watch_dropped;
static bool axis_watch_installed;

static bool SDLCALL axis_watch_callback(void *userdata, SDL_Event *event)
{
    (void)userdata;
    if (event->type != SDL_EVENT_GAMEPAD_AXIS_MOTION ||
        event->gaxis.which != axis_watch_gamepad ||
        event->gaxis.axis != axis_watch_axis) {
        return true;
    }
    SDL_LockMutex(axis_watch_mutex);
    if (axis_watch_count == AXIS_WATCH_CAPACITY) {
        axis_watch_dropped++;
    } else {
        Uint32 tail = (axis_watch_head + axis_watch_count) % AXIS_WATCH_CAPACITY;
        axis_watch_values[tail] = event->gaxis.value;
        axis_watch_count++;
    }
    SDL_UnlockMutex(axis_watch_mutex);
    return true;
}

static int axis_watch_start(SDL_JoystickID gamepad, Uint8 axis)
{
    if (!axis_watch_mutex) {
        axis_watch_mutex = SDL_CreateMutex();
        if (!axis_watch_mutex) {
            return 0;
        }
    }
    if (axis_watch_installed) {
        SDL_RemoveEventWatch(axis_watch_callback, NULL);
    }
    SDL_LockMutex(axis_watch_mutex);
    axis_watch_gamepad = gamepad;
    axis_watch_axis = axis;
    axis_watch_head = 0;
    axis_watch_count = 0;
    axis_watch_dropped = 0;
    SDL_UnlockMutex(axis_watch_mutex);
    axis_watch_installed = SDL_AddEventWatch(axis_watch_callback, NULL);
    return axis_watch_installed ? 1 : 0;
}

static Uint32 axis_watch_poll(void)
{
    Uint32 result = 0;
    SDL_LockMutex(axis_watch_mutex);
    if (axis_watch_count != 0) {
        result = 0x10000u | (Uint16)axis_watch_values[axis_watch_head];
        axis_watch_head = (axis_watch_head + 1) % AXIS_WATCH_CAPACITY;
        axis_watch_count--;
    }
    SDL_UnlockMutex(axis_watch_mutex);
    return result;
}

static Uint64 axis_watch_stop(void)
{
    if (axis_watch_installed) {
        SDL_RemoveEventWatch(axis_watch_callback, NULL);
        axis_watch_installed = false;
    }
    SDL_LockMutex(axis_watch_mutex);
    Uint64 dropped = axis_watch_dropped;
    axis_watch_head = 0;
    axis_watch_count = 0;
    SDL_UnlockMutex(axis_watch_mutex);
    return dropped;
}
*/
import "C"

// StartGamepadAxisWatch installs a fixed C-side event ring. The event callback
// can run on SDL's device thread without calling into Go or allocating.
func StartGamepadAxisWatch(gamepad GamepadID, axis GamepadAxis) error {
	if C.axis_watch_start(C.SDL_JoystickID(gamepad), C.Uint8(axis)) == 0 {
		return GetError()
	}
	return nil
}

// PollWatchedGamepadAxis returns the oldest captured value.
func PollWatchedGamepadAxis() (int16, bool) {
	result := uint32(C.axis_watch_poll())
	return int16(uint16(result)), result&0x10000 != 0
}

// StopGamepadAxisWatch removes the watch and returns the number of values lost
// because the fixed ring was full.
func StopGamepadAxisWatch() uint64 {
	return uint64(C.axis_watch_stop())
}
