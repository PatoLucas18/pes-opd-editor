//go:build windows

// Cuentagotas de Windows para el .exe (WebView2 no expone window.EyeDropper).
// Lee el color de la pantalla bajo el cursor con GetPixel y lo entrega a la página
// como un polyfill de window.EyeDropper. Sólo se usa en el programa compilado;
// los HTML no cambian (en localhost sigue el cuentagotas nativo del navegador).

package main

import (
	"encoding/json"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	eyeUser32            = syscall.NewLazyDLL("user32.dll")
	eyeGdi32             = syscall.NewLazyDLL("gdi32.dll")
	procGetCursorPos     = eyeUser32.NewProc("GetCursorPos")
	procGetDC            = eyeUser32.NewProc("GetDC")
	procReleaseDC        = eyeUser32.NewProc("ReleaseDC")
	procGetAsyncKeyState = eyeUser32.NewProc("GetAsyncKeyState")
	procGetPixel         = eyeGdi32.NewProc("GetPixel")
)

type eyePoint struct{ X, Y int32 }

// eyeState es lo que la página recibe en cada sondeo.
type eyeState struct {
	OK   bool   `json:"ok"`
	Hex  string `json:"hex"`
	Down bool   `json:"down"`
	Esc  bool   `json:"esc"`
}

// sampleScreenHex devuelve el color "#rrggbb" del píxel bajo el cursor.
func sampleScreenHex() (string, bool) {
	var p eyePoint
	if ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); ret == 0 {
		return "", false
	}
	hdc, _, _ := procGetDC.Call(0) // DC de toda la pantalla
	if hdc == 0 {
		return "", false
	}
	defer procReleaseDC.Call(0, hdc)
	c, _, _ := procGetPixel.Call(hdc, uintptr(p.X), uintptr(p.Y))
	if c == 0xFFFFFFFF { // CLR_INVALID
		return "", false
	}
	// COLORREF = 0x00BBGGRR
	r := uint8(c)
	g := uint8(c >> 8)
	b := uint8(c >> 16)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b), true
}

func eyeKeyDown(vk uintptr) bool {
	ret, _, _ := procGetAsyncKeyState.Call(vk)
	return ret&0x8000 != 0
}

// ssPickSample la llama la página (window.ssPickSample) cada ~30 ms mientras
// el cuentagotas está abierto. Devuelve JSON como texto.
func ssPickSample() string {
	var st eyeState
	st.Hex, st.OK = sampleScreenHex()
	st.Down = eyeKeyDown(0x01) // VK_LBUTTON
	st.Esc = eyeKeyDown(0x1B)  // VK_ESCAPE
	out, _ := json.Marshal(st)
	return string(out)
}

// eyeDropJS se inyecta antes de cargar la página. Si ya existe el cuentagotas
// nativo, no hace nada.
const eyeDropJS = `
(function () {
  if (window.EyeDropper) return;
  function EyeDropper() {}
  EyeDropper.prototype.open = function (opts) {
    var signal = opts && opts.signal;
    return new Promise(function (resolve, reject) {
      var primed = false, wasDown = false, busy = false, timer = null, done = false;
      function stop() { done = true; if (timer) clearInterval(timer); }
      if (signal) {
        signal.addEventListener('abort', function () {
          if (done) return;
          stop();
          reject(new DOMException('Cancelado', 'AbortError'));
        });
      }
      timer = setInterval(function () {
        if (done || busy || typeof window.ssPickSample !== 'function') return;
        busy = true;
        Promise.resolve(window.ssPickSample()).then(function (raw) {
          busy = false;
          if (done) return;
          var s = typeof raw === 'string' ? JSON.parse(raw) : raw;
          if (!s || !s.ok) return;
          if (s.esc) {
            stop();
            reject(new DOMException('Cancelado', 'AbortError'));
            return;
          }
          // Sólo cuenta un clic nuevo (botón pasa de suelto a apretado).
          if (!primed) { primed = true; wasDown = !!s.down; return; }
          if (s.down && !wasDown) {
            stop();
            resolve({ sRGBHex: s.hex });
            return;
          }
          wasDown = !!s.down;
        }, function (e) {
          busy = false;
          stop();
          reject(e);
        });
      }, 30);
    });
  };
  window.EyeDropper = EyeDropper;
})();
`
