# OPD Editor

Editor de los gráficos 2D de **PES 6** (`enter.opd`, `game2d.opd`, `scoreboard.bin`, `pk.opd`). Estado: **v0.1.1-alpha**.

Es una sola página HTML sin dependencias, con un servidor local opcional (PowerShell) y un programa de escritorio opcional (Go + WebView2).

## Qué hace
- Abre archivos `.opd` / `.bin` sueltos o una carpeta completa (OPD + `textures/`), y el `.cfg` con nombres y colores.
- Muestra escenas, bloques, texturas, textos e iconos sobre el área real de pantalla (X ±4096, Y ±3584), en 4:3 o 16:9.
- Mover, escalar y rotar objetos y vértices estilo Blender (`G`, `S`, `R`, `A`, `X`/`Y`), con imán entre vértices, rejilla, deshacer y rehacer.
- Modo edición y modo creación (agregar, duplicar y eliminar objetos).
- Vista UV de cada textura, recarga de PNG, fondo guía por escena (por ejemplo una captura del juego) y cuentagotas de color.
- Guardar y «Guardar como…»; las texturas de los atlas se leen de `textures/<nombre>.png`.

## Uso rápido (Windows)
1. Descargar o clonar este repositorio.
2. Ejecutar `app/EditorOPD-run.bat` (abre el editor en el navegador, en `http://localhost:878x/`).
3. **Abrir carpeta (OPD + textures)** y elegir la carpeta del marcador o del `.opd`.

Sin servidor también funciona: abrir `app/EditorOPD.html` en el navegador (Chrome, Edge o Firefox actuales); algunos botones de archivo se limitan a lo que permite el navegador.

Hacer copia de seguridad de los archivos antes de editar (el servidor crea un `.bak` la primera vez que guarda).

## Programa de escritorio (opcional)
En `desktop/` está el código en Go para compilar `EditorOPD.exe` (ventana propia, requiere Microsoft Edge WebView2 Runtime y Go 1.26 o posterior). Ver `desktop/LEEME.txt`. El `.exe` no se incluye en el repositorio.

## Estructura
```
app/      EditorOPD.html + servidor local (EditorOPD-Servidor.ps1, EditorOPD-run.bat)
desktop/  código fuente del programa de escritorio (Go + WebView2)
docs/     formato OPD de PES 6
```

## Documentación
- [`docs/01-formato-opd.md`](docs/01-formato-opd.md): contenedor, bloques, texturas, textos, iconos, escenas, unidades y rangos.

## Proyectos relacionados
- `pes-dpow-editor`: el mismo tipo de editor para el formato DPOW de PES 2008–2010.
- `pes-afp-editor`: editor del HUD 2D de PES 2016–2018.

## Créditos y aviso
Autores: **Pato_Lucas18** y **Claude** (Anthropic). No incluye archivos ni texturas de ningún juego y no está afiliado a Konami. Pro Evolution Soccer y Winning Eleven son marcas de sus propietarios.

Licencia: [MIT](LICENSE).
