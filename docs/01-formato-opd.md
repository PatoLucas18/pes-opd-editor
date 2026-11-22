# Formato OPD de PES 6 (`enter.opd`, `game2d.opd`, `scoreboard.bin`, `pk.opd`)

Resumen del formato que lee y escribe OPD Editor. Todo es *little endian*. Los offsets internos son relativos al inicio de los datos descomprimidos y múltiplos de 4.

## Contenedor
- **Cabecera de 32 bytes:**
  - dword0: magic (`00 0E 01 00` en OPD; `0x00010400` en `scoreboard.bin` y `pk.opd`);
  - dword1: tamaño del flujo zlib;
  - dword2: tamaño descomprimido;
  - el resto, ceros.
- Después va el flujo zlib (empieza con `78 DA`) y relleno con ceros hasta múltiplo de 16.

## Cabecera del OPD descomprimido (16 bytes)
`OffNombres, OffBloques, OffEscenas, OffWorkKeys` (dword cada uno; 0 = no hay). Cada tabla es `{count, offsetLista}` y la lista son `count` dwords con offsets.
- **Nombres:** cadenas terminadas en 0 con los nombres de las texturas (atlas PNG en `textures/<nombre>.png`). Ejemplos: `enter`, `common_wind`, `game_01`.
- **WorkKeys:** etiquetas de 4 bytes (`wkm`, `wkb`, `wks`).

`scoreboard.bin` y `pk.opd` usan una cabecera de solo 8 bytes `{OffNombres, OffBloques}`: no tienen escenas ni WorkKeys.

## Bloque (24 bytes)
| Off | Tipo | Campo |
|---|---|---|
| 0 | u8 | cantidad de texturas |
| 1 | u8 | cantidad de textos |
| 2 | u8 | cantidad de iconos |
| 3 | u8 | desconocido (en `pk.opd` marca la versión: 2) |
| 4 / 6 | i16 | PosX / PosY del bloque |
| 8 | i16 | Overlap (Z) |
| 10 | i16 | desconocido |
| 12 / 16 / 20 | u32 | offset a la lista de texturas / a los textos / a los iconos |

En `scoreboard.bin` el bloque mide 16 bytes: no tiene posición ni overlap.

## Textura (cabecera de 28 bytes; es una tira de vértices que recorta el atlas)
| Off | Tipo | Campo |
|---|---|---|
| 0 | u8 | desconocido |
| 1 | u8 | desconocido |
| 2 | u8 | UseVertexColor (0/1; en `.bin` se vio 9) |
| 3 | u8 | TextureIndex (índice en la tabla de nombres) |
| 4 | i16 | cantidad de vértices |
| 6..9 | u8×4 | RGBA base |
| 10 / 12 | i16 | PosX / PosY |
| 14 | i16 | Overlap (Z) |
| 16 | u32 | offset a los vértices: i16 X, Y, Z (6 bytes c/u) |
| 20 | u32 | offset a los UV: i16 U, V (4 bytes c/u) |
| 24 | u32 | offset a los colores por vértice: RGBA (4 bytes c/u), solo si UseVertexColor |

En `scoreboard.bin` la textura es un registro fijo de 46 bytes: posición +4/+6, overlap +8, tamaño +10/+12, 4 UV en +14 (orden TL, TR, BL, BR) y 4 colores RGBA.

## Texto (24 bytes)
| Off | Tipo | Campo |
|---|---|---|
| 0 | u8 | desconocido (1) |
| 1 | u8 | FontIndex |
| 2 | u8 | Alineación (bits, ver abajo) |
| 3 | u8 | desconocido |
| 4 / 6 | i16 | PosX / PosY |
| 8 | i16 | Overlap |
| 10 / 12 | i16 | Ancho / Alto de la caja |
| 14 | u8 | StyleMode |
| 15 | u8 | ColorIndex (paleta) |
| 16 | i16 | **ID** (el juego busca el texto por este id) |
| 18 | i16 | ID2 |
| 20 | i16 | StringID |
| 22 | i16 | BlockID (bloque de la tabla de textos `s_text`; clave = BlockID<<16 \| StringID) |

## Icono (14 bytes): rectángulo con nombre, sin gráfico propio
| Off | Tipo | Campo |
|---|---|---|
| 0 | u8 | desconocido (2) |
| 1 | u8 | desconocido |
| 2 | i16 | **ID** |
| 4 / 6 | i16 | PosX / PosY |
| 8 | i16 | Overlap |
| 10 / 12 | i16 | Ancho / Alto |

El juego pide el icono por ID para saber dónde dibujar algo: escudos, banderas, nombres de jugador, puntos de referencia, etc.

## Escena (solo OPD)
| Off | Tipo | Campo |
|---|---|---|
| 0 | u16 | cantidad de bloques |
| 2 | u8 | desconocido (1) |
| 3 | u8 | cantidad de texturas |
| 4 | u32 | offset a la lista de bloques (4 bytes por entrada: u16 índice de bloque + u16 desconocido) |
| 8 | u32 | offset a la lista de texturas (**u16, paso 2**) |

- Una escena es un conjunto de bloques, que son variantes de layout de la misma pantalla.
- Los ids de textos e iconos solo son únicos dentro de una escena.
- La lista de texturas de la escena tiene que incluir todos los `TextureIndex` que usan sus bloques. Si falta alguno, en PS2 falla.

## Unidades y rangos
- **Posiciones y tamaños:** int16 en **1/16 de unidad** (512 = 32 unidades). Pantalla aproximada: X de −4096 a 4096, Y de −3584 a 3584, con el eje Y hacia abajo. 8192 = fuera de pantalla (oculto).
- **UV:** 0 a 4096 = el ancho o alto completo de la textura.
- **Overlap = profundidad Z:** se suman la Z del bloque, la del objeto y la de cada vértice. El juego la limita a 50..16000. **Menor overlap = más adelante** (se dibuja encima). Con valores altos se pierde precisión: a 16000 solo se distinguen saltos de unas 77 unidades.
- **Colores:** RGB 0..255. **Alfa de OPD: 0..128 (128 = opaco, estilo PS2).** En `scoreboard.bin` el alfa va de 0 a 255.
- **Alineación del texto (byte +2):** bits 0-2 horizontal (0 izquierda, 1 centro, 2 derecha, 3 reparte espacio entre letras); bit 3 autoajuste; bits 4-5 vertical. Valores típicos: 24 = izquierda, 25 = centro, 26 = derecha (con autoajuste y vertical 1).
- **StyleMode:** 0 = texto plano con el color de la paleta; distinto de 0 = texto con borde (relleno de la paleta y borde negro, o blanco si el relleno es negro). Solo funciona si la fuente es del formato 0x10.
- **ColorIndex:** paleta de 13 posiciones (0 a 12). 0 `FFFFFF`, 1/2/3/5 `000000`, 4 `646464`, 6 `F0F0F0`, 7 `B4FF00`, 8 `F0FF30`, 9 `FFBA30`, 10 `FF785C`, 11 `1464F0`; 12 = invisible (alfa 0); 13 o más: sin color.
- **IDs:** int16 (en la práctica 0 a 511). Los bloques de `s_text` del juego van de 1 a 1000.

## Notas del motor (`pes6.exe`)
- El juego **solo crea los textos de su propia tabla de ids de cada escena**. Un texto o icono con un id nuevo no aparece salvo que un mod lo cree.
- `OpdFindEntry(opd, tipo, id)` en `0x95C900`: tipo 0 = textura por índice, 1 = texto por id, 2 = icono por id.
- `OpdGetIconRect` en `0x950490`: devuelve x, y, w, h en unidades.

## Relacionado
El formato DPOW de PES 2008–2010 es un OPD con cabecera ampliada: ver el repositorio `pes-dpow-editor`.
