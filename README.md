# Rev Visión — Actualizaciones automáticas

Canal exclusivo para **Rev Visión · Óptica y Salud Visual**, independiente de Recepción e Historia Clínica del Dr. Revelo.

## Estado comprobado

- **Versión pública activa: 0.4.0**, según `update.json`.
- **Firma privada en GitHub:** comprobada correctamente por [Validar secreto de firma](.github/workflows/verify-signing-key.yml). Nunca se guarda en este repositorio.
- **Compilación y publicación automáticas:** [build-and-publish.yml](.github/workflows/build-and-publish.yml) ya configurado.
- **Importación inicial de fuentes 0.4.2:** [import-source.yml](.github/workflows/import-source.yml) ya configurado, pendiente de colocar el único archivo ZIP inicial en `bootstrap/`.

## Primera incorporación (una sola vez)

Subir `RevVision_Fuentes_0.4.2_SUBIR_UNA_VEZ.zip` dentro de [bootstrap](bootstrap/). El flujo comprobará su SHA-256 fijo, extraerá las fuentes a `src/`, eliminará el ZIP del árbol de trabajo y ejecutará la compilación de la 0.4.2.

Después, los cambios futuros se harán sobre el código de `src/`. Cambiar `src/VERSION` por una versión superior disparará automáticamente el compilador.

## Seguridad de las publicaciones

1. El sistema exige el código completo y ejecuta pruebas Go/JS.
2. Comprueba que el secreto Ed25519 corresponda a la [clave pública](keys/release_public.pem).
3. El script del proyecto genera ejecutables, ZIP, versiones y hashes automáticamente desde `src/VERSION`.
4. GitHub Actions crea y adjunta la Release sin que sea necesario subir ZIP manualmente.
5. [verify_release.py](.github/scripts/verify_release.py) verifica el ZIP descargado, su firma y sus SHA.
6. Sólo después se actualiza `update.json`; si falla algo, el canal anterior permanece activo.

**Nunca subir:** historias clínicas reales, bases de datos, credenciales, contraseñas, claves privadas ni archivos `.env`.

## Limitaciones

El cambio de versión del launcher requiere pruebas reales en Windows. Los datos clínicos siguen siendo locales por computadora, sin sincronización ni autorización para operar con historias clínicas reales. La primera compilación automática aún está pendiente hasta recibir las fuentes.
