# Rev Visión — Canal de actualizaciones

Repositorio independiente del software **Rev Visión · Óptica y Salud Visual**. No contiene pacientes, historias clínicas, abonos, bases de datos ni credenciales.

## Estado actual

- **Versión activa del canal: 0.4.0**, verificada en `update.json`.
- **Versión 0.4.1: candidato en pruebas locales, todavía NO publicado**.
- La versión 0.4.0 del launcher no admite el nuevo formato de actualización del propio launcher. El paso a 0.4.1 requiere una instalación inicial con el instalador de transición, después de la prueba Windows.
- A partir de un launcher 0.4.1 validado, el formato firmado 2 permite entregar aplicación, launcher y auxiliar juntos.

## Blindaje de publicación

Al publicar una GitHub Release, el flujo [Verificar y activar](.github/workflows/activate-release.yml) ejecuta [el verificador](.github/scripts/verify_release.py), que comprueba:

1. Versión y URL fija del paquete del repositorio.
2. Firma **Ed25519** utilizando solo la [clave pública](keys/release_public.pem).
3. SHA-256 de ZIP, aplicación y, para formato 2, también launcher y auxiliar.
4. Archivos exactos del ZIP y versiones en `release.json`.
5. Ausencia de retroceso respecto de la versión activa.

**Sólo si pasa todo**, el flujo modifica `update.json`. El archivo candidato por sí solo no publica una actualización. Está prohibido subir claves privadas, datos clínicos o copias de bases de datos.

## Fuente única de versiones

`VERSION` en el código fuente es la única fuente para la compilación. `build_release.sh` genera nombres de archivo, hashes y manifiesto firmado sin copiar valores manualmente.

## Limitaciones

El sistema continúa en pruebas locales y no sincroniza pacientes entre computadoras. El instalador nuevo y el autocambio del launcher requieren comprobación en Windows real antes de declarar producción.
