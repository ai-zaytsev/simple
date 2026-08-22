# Keep the engine binding surface: it is called across the JNI boundary and
# is not reachable from Kotlin call graphs that R8 can see.
-keep class go.** { *; }
-keep class libXray.** { *; }
-keep class xray.** { *; }
