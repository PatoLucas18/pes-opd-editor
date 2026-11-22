//go:build windows

// Editor OPD como programa de escritorio (Go + WebView2), igual que ModulosConfig.exe.
// Sirve EditorOPD.html desde un puerto local (solo 127.0.0.1) para que el navegador
// integrado permita abrir y guardar archivos y carpetas (File System Access API).
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	webview "github.com/jchv/go-webview2"
)

//go:embed EditorOPD.html
var embedded []byte

const appTitle = "Editor OPD"

func main() {
	exe, err := os.Executable()
	if err != nil {
		showError("No pude ubicar EditorOPD.exe:\r\n" + err.Error())
		return
	}
	exeDir := filepath.Dir(exe)
	logFile, _ := os.OpenFile(filepath.Join(exeDir, "EditorOPD.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if logFile != nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	listener, address, err := localListener()
	if err != nil {
		showError("No pude iniciar el servidor local:\r\n" + err.Error())
		return
	}
	// Si hay un EditorOPD.html junto al exe se usa ese (permite actualizar el editor sin recompilar);
	// si no, se usa la copia incrustada al compilar.
	external := filepath.Join(exeDir, "EditorOPD.html")
	mux := http.NewServeMux()
	// API de archivos para el editor (carpeta por ruta). Solo acepta pedidos locales del mismo origen.
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r, listener.Addr().String()) {
			return
		}
		_, _ = w.Write([]byte("editoropd"))
	})
	pick := func(folder bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !localOnly(w, r, listener.Addr().String()) {
				return
			}
			if r.Method != http.MethodPost {
				http.Error(w, "método no permitido", 405)
				return
			}
			chosen := ""
			if folder {
				if chosen = pickFolder(); chosen != "" {
					allowDir(chosen)
				}
			} else {
				if chosen = pickFile(r.URL.Query().Get("start")); chosen != "" {
					allowDir(filepath.Dir(chosen))
				}
			}
			if chosen == "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]string{"path": chosen})
		}
	}
	mux.HandleFunc("/api/pick", pick(false))
	mux.HandleFunc("/api/pickdir", pick(true))
	mux.HandleFunc("/api/list", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r, listener.Addr().String()) {
			return
		}
		dir := filepath.Clean(r.URL.Query().Get("p"))
		if !filepath.IsAbs(dir) || !inRoot(dir) {
			http.Error(w, "ruta no válida", 400)
			return
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		files, dirs := []string{}, []string{}
		for _, e := range ents {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			} else {
				files = append(files, e.Name())
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string][]string{"files": files, "dirs": dirs})
	})
	mux.HandleFunc("/api/read", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r, listener.Addr().String()) {
			return
		}
		p := filepath.Clean(r.URL.Query().Get("p"))
		if !filepath.IsAbs(p) || !inRoot(p) || !readExt[strings.ToLower(filepath.Ext(p))] {
			http.Error(w, "archivo no permitido", 400)
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "no existe", 404)
			} else {
				http.Error(w, err.Error(), 500)
			}
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/api/write", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r, listener.Addr().String()) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "método no permitido", 405)
			return
		}
		p := filepath.Clean(r.URL.Query().Get("p"))
		if !filepath.IsAbs(p) || !inRoot(p) || !writeExt[strings.ToLower(filepath.Ext(p))] {
			http.Error(w, "archivo no permitido", 400)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, e := os.Stat(p); e == nil {
			if _, e2 := os.Stat(p + ".bak"); os.IsNotExist(e2) {
				if orig, e3 := os.ReadFile(p); e3 == nil {
					_ = os.WriteFile(p+".bak", orig, 0644)
				}
			}
		}
		if err := os.WriteFile(p, data, 0644); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Registro de diagnóstico que manda la página (errores de JavaScript, archivos abiertos, selector de archivos).
	mux.HandleFunc("/client-log", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, listener.Addr().String()) || !isLoopback(r.RemoteAddr) {
			http.Error(w, "acceso local solamente", 403)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "método no permitido", 405)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		log.Printf("pagina: %s", strings.TrimSpace(string(body)))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, listener.Addr().String()) || !isLoopback(r.RemoteAddr) {
			http.Error(w, "acceso local solamente", 403)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "método no permitido", 405)
			return
		}
		if r.URL.Path != "/" && r.URL.Path != "/index.html" && r.URL.Path != "/EditorOPD.html" {
			http.NotFound(w, r)
			return
		}
		data := embedded
		if b, e := os.ReadFile(external); e == nil && len(b) > 0 {
			data = b
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if e := server.Serve(listener); e != nil && e != http.ErrServerClosed {
			log.Printf("servidor local: %v", e)
		}
	}()
	dataDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "EditorOPD", "WebView2")
	if os.Getenv("LOCALAPPDATA") == "" {
		dataDir = filepath.Join(os.TempDir(), "EditorOPD-WebView2")
	}
	if err = os.MkdirAll(dataDir, 0700); err != nil {
		_ = server.Close()
		showError("No pude preparar los datos de WebView2:\r\n" + err.Error())
		return
	}
	view := webview.NewWithOptions(webview.WebViewOptions{
		Debug: false, DataPath: dataDir, AutoFocus: true,
		WindowOptions: webview.WindowOptions{Title: appTitle, Width: 1500, Height: 950, Center: true},
	})
	if view == nil {
		_ = server.Close()
		showError("No se pudo iniciar WebView2. Instala Microsoft Edge WebView2 Runtime y vuelve a abrir Editor OPD.")
		return
	}
	defer view.Destroy()
	ownerHwnd = uintptr(view.Window())
	view.Init(eyeDropJS)
	_ = view.Bind("ssPickSample", ssPickSample)
	defer server.Close()
	src := "incrustado"
	if _, e := os.Stat(external); e == nil {
		src = external
	}
	log.Printf("iniciado: url=%s html=%s", address, src)
	view.Navigate(address)
	view.Run()
}

// Puertos 8791-8799: distintos de ModulosConfig (8787+) no hace falta, pero sí de "EditorOPD run.bat" (8780-8790).
func localListener() (net.Listener, string, error) {
	for port := 8791; port < 8800; port++ {
		l, e := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if e == nil {
			return l, "http://" + l.Addr().String() + "/", nil
		}
	}
	return nil, "", fmt.Errorf("los puertos locales 8791–8799 están ocupados")
}

var readExt = map[string]bool{".opd": true, ".bin": true, ".cfg": true, ".csv": true, ".png": true}   // .csv = string.csv (textos del juego)
var writeExt = map[string]bool{".opd": true, ".bin": true, ".cfg": true, ".csv": true}

// localOnly: solo pedidos desde esta misma PC y desde la propia página (no desde otros sitios).
func localOnly(w http.ResponseWriter, r *http.Request, addr string) bool {
	if !strings.EqualFold(r.Host, addr) || !isLoopback(r.RemoteAddr) {
		http.Error(w, "acceso local solamente", 403)
		return false
	}
	if o := r.Header.Get("Origin"); o != "" && o != "http://"+addr {
		http.Error(w, "origen no permitido", 403)
		return false
	}
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" && s != "same-origin" && s != "none" {
		http.Error(w, "origen no permitido", 403)
		return false
	}
	return true
}

// Carpetas elegidas con los diálogos de Windows: solo ahí se lee y se guarda.
var (
	allowedMu   sync.Mutex
	allowedDirs = map[string]bool{}
	ownerHwnd   uintptr
)

func allowDir(dir string) {
	allowedMu.Lock()
	allowedDirs[strings.ToLower(filepath.Clean(dir))] = true
	allowedMu.Unlock()
}

func inRoot(p string) bool {
	lp := strings.ToLower(filepath.Clean(p))
	allowedMu.Lock()
	defer allowedMu.Unlock()
	for root := range allowedDirs {
		if lp == root || strings.HasPrefix(lp, strings.TrimRight(root, `\`)+`\`) {
			return true
		}
	}
	return false
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

// pickFile muestra el diálogo "Abrir" de Windows. Devuelve "" si se cancela.
func pickFile(startDir string) string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	buf := make([]uint16, 4096)
	filter := utf16.Encode([]rune("OPD / scoreboard.bin (*.opd;*.bin)\x00*.opd;*.bin\x00Todos los archivos (*.*)\x00*.*\x00\x00"))
	title, _ := syscall.UTF16PtrFromString("Abrir .opd / scoreboard.bin")
	ofn := openFileName{hwndOwner: ownerHwnd, lpstrFilter: &filter[0], lpstrFile: &buf[0], nMaxFile: uint32(len(buf)), lpstrTitle: title,
		flags: 0x00080000 | 0x00001000 | 0x00000800 | 0x00000004 | 0x00000008} // EXPLORER | FILEMUSTEXIST | PATHMUSTEXIST | HIDEREADONLY | NOCHANGEDIR
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	if st, e := os.Stat(startDir); startDir != "" && e == nil && st.IsDir() {
		if d, e2 := syscall.UTF16PtrFromString(startDir); e2 == nil {
			ofn.lpstrInitialDir = d
		}
	}
	r, _, _ := syscall.NewLazyDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&ofn)))
	runtime.KeepAlive(filter)
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

type browseInfo struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

// pickFolder muestra el diálogo de carpetas de Windows. Devuelve "" si se cancela.
func pickFolder() string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole32 := syscall.NewLazyDLL("ole32.dll")
	shell32 := syscall.NewLazyDLL("shell32.dll")
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2) // apartamento de un solo hilo
	if hr == 0 || hr == 1 {
		defer ole32.NewProc("CoUninitialize").Call()
	}
	display := make([]uint16, 520)
	title, _ := syscall.UTF16PtrFromString("Carpeta del marcador (la que tiene los .opd y la subcarpeta textures)")
	bi := browseInfo{hwndOwner: ownerHwnd, pszDisplayName: &display[0], lpszTitle: title, ulFlags: 0x0001 | 0x0040 | 0x0200} // solo carpetas | estilo nuevo | sin "nueva carpeta"
	pidl, _, _ := shell32.NewProc("SHBrowseForFolderW").Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer ole32.NewProc("CoTaskMemFree").Call(pidl)
	buf := make([]uint16, 4096)
	ok, _, _ := shell32.NewProc("SHGetPathFromIDListW").Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func isLoopback(remote string) bool {
	host, _, e := net.SplitHostPort(remote)
	if e != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func showError(message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString(appTitle)
	proc.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
