# Rev Visión — Actualizaciones de escritorio

Canal exclusivo para **Rev Visión — Óptica y Salud Visual**. No es el sistema de Recepción ni Historia Clínica del Dr. Revelo.

## Estado

**Versión 0.4.0 preparada, pendiente de su primera GitHub Release.** El launcher no descargará actualizaciones por Internet hasta que exista un paquete público verificado y un archivo `update.json` válido en la rama `main`.

## Cómo activar la primera publicación

1. Abrir **[Crear nueva Release](https://github.com/fanserick-star/revvision-updates/releases/new)**.
2. Introducir etiqueta exacta `v0.4.0`; título `Rev Visión 0.4.0`.
3. Adjuntar exactamente `RevVision_App_0.4.0_ACTUALIZACION.zip` (aprox. 3,1 MB), **no el instalador ni el código fuente**.
4. Pulsar **Publish release**. El flujo [Verificar y activar actualización](.github/workflows/activate-release.yml) comprobará el nombre, la versión, los SHA-256 del ZIP y el EXE y publicará `update.json` solo si todo coincide.
5. Verificar que aparezcan [update.json](https://raw.githubusercontent.com/fanserick-star/revvision-updates/main/update.json) y la [descarga de v0.4.0](https://github.com/fanserick-star/revvision-updates/releases/download/v0.4.0/RevVision_App_0.4.0_ACTUALIZACION.zip).

El manifiesto **firmado** de v0.4.0 está guardado provisionalmente en [candidates/v0.4.0.json](candidates/v0.4.0.json). Tener un candidato no habilita la actualización.

## Verificación inicial

SHA-256 del ZIP: `64a96ae07ab0e8666f344aefd4ef4c2bfc519c0adf126558d5795169c30485dc`.

La verificación digital Ed25519 la realiza el launcher local antes de aplicar una actualización.

## Seguridad y funcionamiento

- **Nunca subir** claves privadas, datos clínicos, inventario de pacientes, bases de datos, copias de seguridad, archivos `.env` ni credenciales.
- El repositorio es **público** únicamente para el software distribuible y manifiestos de actualizaciones.
- Para nuevas versiones se necesita una Release nueva, un ZIP distinto, un manifiesto candidato firmado con la misma clave privada y su comprobación.
- El launcher 0.4.0 **no actualiza si el manifiesto publicado es de la misma versión**. La primera actualización automática real será cuando exista una versión superior publicada y firmada.
- La app de pruebas mantiene una base **local en cada PC**, todavía sin sincronización. No introducir datos clínicos reales antes de implementar controles de seguridad y respaldos.
