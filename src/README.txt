REV VISIÓN 0.4.1 — PRUEBA DE LANZADOR BLINDADO
===================================================
Estado: versión candidata LOCAL; NO publicada en el canal automático todavía.
Canal público actual: 0.4.0. No usar datos reales de pacientes.

OBJETIVO
- Una única fuente VERSION genera automáticamente cuatro EXE/artefactos,
  release.json, el SHA-256 de cada archivo y update.json firmado Ed25519.
- Impedir múltiples launchers/instaladores simultáneos mediante mutex Windows.
- Descarga en carpeta aislada y verificación de firma, versiones y 4 SHA-256.
- Recuperación tras cortes/reinicio durante descarga, renombrado y arranque.
- Confirmación por /api/health con versión correcta y estabilidad de 2,4 s.
- Restauración de última versión validada si el nuevo ejecutable falla.
- Protección de una copia de respaldo; nada de app_patch_1, _2, _3...
- Autoactualización del launcher a partir de la versión nueva mediante
  LauncherUpdater.exe verificado y reemplazo atómico tras cerrar launcher.
- El instalador verifica el paquete empaquetado antes de instalar y conserva
  %APPDATA%\RevVision\Pruebas\data (historias, ventas, inventario, abonos).

POR QUÉ LA 0.4.1 NECESITA INSTALADOR UNA VEZ
El launcher 0.4.0 no entiende el nuevo formato firmado 2 que incluye su
propio launcher + ayudante. No puede reescribirse retroactivamente.
Actualizar 0.4.0 a 0.4.1 con INSTALAR_REVVISION_0.4.1_BLINDADO.exe.
El soporte para AUTOACTUALIZAR EL LAUNCHER opera en versiones posteriores
firmadas con el formato nuevo. No se puede certificar hasta un salto real.

LIMITACIONES IMPORTANTES
- Falta ejecutar instalador y actualizaciones en Windows físico y probar
  cortes abruptos, antivirus, Edge modo aplicación y la impresora CRM-308.
- Barra de progreso visual 0-100 (real) aún requiere pruebas de interfaz.
  Hay mensajes de fallo y registro de actualizaciones, pero no un medidor
  visual certificado equivalente al del launcher de Recepción.
- El EXE no está firmado con certificado Authenticode de Windows (distinto
  de firma Ed25519 de las actualizaciones); SmartScreen puede advertir.
- El sistema de pacientes sigue siendo local por PC. No hay sincronización,
  autenticación clínica robusta ni respaldo externo de producción.

PARA CONSTRUIR SIN HASH ESCRITO A MANO
1. Cambiar solo VERSION al preparar una versión nueva.
2. Mantener la clave Ed25519 privada fuera del proyecto.
3. Exportar REVVISION_SIGNING_KEY apuntando a la ruta de la clave.
4. Ejecutar ./build_release.sh <directorio-de-salida> en Linux/macOS con Go.
5. Verificar la Release usando .github/scripts/verify_release.py de GitHub.
6. Publicar ZIP de esa versión; GitHub activa update.json SOLO si pasa
   firma y todos los SHA. Nunca publicar claves privadas ni bases clínicas.
