package sdl

/*
#cgo CFLAGS: -I${SRCDIR}/../deps/SDL/include

#include <stdlib.h>

#include <SDL3/SDL_audio.h>
*/
import "C"

import "unsafe"

// AudioDeviceID identifies a physical or virtual SDL audio endpoint.
type AudioDeviceID uint32

// AudioFormat identifies the sample representation used by an AudioSpec.
type AudioFormat uint16

const (
	AudioS16 AudioFormat = C.SDL_AUDIO_S16
)

// AudioSpec describes application-side audio presented to an SDL stream.
type AudioSpec struct {
	Format   AudioFormat
	Channels int
	Freq     int
}

// AudioStream is an SDL stream bound to an audio device.
type AudioStream struct {
	cStream *C.SDL_AudioStream
}

func audioDevices(recording bool) ([]AudioDeviceID, error) {
	var count C.int
	var devices *C.SDL_AudioDeviceID
	if recording {
		devices = C.SDL_GetAudioRecordingDevices(&count)
	} else {
		devices = C.SDL_GetAudioPlaybackDevices(&count)
	}
	if devices == nil {
		if count == 0 {
			return []AudioDeviceID{}, nil
		}
		return nil, GetError()
	}
	defer C.SDL_free(unsafe.Pointer(devices))

	cDevices := unsafe.Slice(devices, int(count))
	result := make([]AudioDeviceID, len(cDevices))
	for index, device := range cDevices {
		result[index] = AudioDeviceID(device)
	}
	return result, nil
}

// GetAudioPlaybackDevices returns the currently visible output endpoints.
func GetAudioPlaybackDevices() ([]AudioDeviceID, error) {
	return audioDevices(false)
}

// GetAudioRecordingDevices returns the currently visible input endpoints.
func GetAudioRecordingDevices() ([]AudioDeviceID, error) {
	return audioDevices(true)
}

// GetAudioDeviceName returns SDL's name for an audio endpoint.
func GetAudioDeviceName(device AudioDeviceID) string {
	name := C.SDL_GetAudioDeviceName(C.SDL_AudioDeviceID(device))
	if name == nil {
		return ""
	}
	return C.GoString(name)
}

// OpenAudioDeviceStream opens a callback-free stream. Playback callers push
// data with Put; recording callers pull data with Get.
func OpenAudioDeviceStream(device AudioDeviceID, spec AudioSpec) (*AudioStream, error) {
	cSpec := C.SDL_AudioSpec{
		format:   C.SDL_AudioFormat(spec.Format),
		channels: C.int(spec.Channels),
		freq:     C.int(spec.Freq),
	}
	stream := C.SDL_OpenAudioDeviceStream(C.SDL_AudioDeviceID(device), &cSpec, nil, nil)
	if stream == nil {
		return nil, GetError()
	}
	return &AudioStream{cStream: stream}, nil
}

// Resume starts the endpoint bound to the stream.
func (s *AudioStream) Resume() error {
	if s == nil || s.cStream == nil {
		return &SDLError{eStr: "invalid audio stream"}
	}
	if !C.SDL_ResumeAudioStreamDevice(s.cStream) {
		return GetError()
	}
	return nil
}

// Put queues application audio for playback.
func (s *AudioStream) Put(data []byte) error {
	if s == nil || s.cStream == nil {
		return &SDLError{eStr: "invalid audio stream"}
	}
	if len(data) == 0 {
		return nil
	}
	if !C.SDL_PutAudioStreamData(s.cStream, unsafe.Pointer(&data[0]), C.int(len(data))) {
		return GetError()
	}
	return nil
}

// Available reports bytes ready to be pulled from a recording stream.
func (s *AudioStream) Available() int {
	if s == nil || s.cStream == nil {
		return 0
	}
	return int(C.SDL_GetAudioStreamAvailable(s.cStream))
}

// Get reads up to len(destination) bytes from a recording stream.
func (s *AudioStream) Get(destination []byte) (int, error) {
	if s == nil || s.cStream == nil {
		return 0, &SDLError{eStr: "invalid audio stream"}
	}
	if len(destination) == 0 {
		return 0, nil
	}
	n := int(C.SDL_GetAudioStreamData(s.cStream, unsafe.Pointer(&destination[0]), C.int(len(destination))))
	if n < 0 {
		return 0, GetError()
	}
	return n, nil
}

// Close destroys a stream and closes its bound endpoint.
func (s *AudioStream) Close() {
	if s == nil || s.cStream == nil {
		return
	}
	C.SDL_DestroyAudioStream(s.cStream)
	s.cStream = nil
}
