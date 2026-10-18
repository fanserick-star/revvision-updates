# Preparación inicial de fuentes de Rev Visión

**Única subida inicial** (no es necesario repetirla para cada actualización).

Desde esta carpeta, selecciona **Add file → Upload files** y sube el archivo exacto `RevVision_Fuentes_0.4.2_SUBIR_UNA_VEZ.zip` que se preparó para la óptica. Debe quedar en `bootstrap/RevVision_Fuentes_0.4.2_SUBIR_UNA_VEZ.zip`.

GitHub Actions verificará su SHA-256 fijado, estructura, versión y ausencia de rutas no permitidas; lo importará bajo `src/`, lo retirará del árbol de trabajo y solicitará una compilación automática. Si las pruebas de compilación, firma y descarga fallan, `update.json` permanecerá sin cambios.

No subir bases de datos, pacientes, claves privadas, archivos .env ni archivos ajenos al paquete.
