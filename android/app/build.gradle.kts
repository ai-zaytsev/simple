plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "download.simplevpn"
    compileSdk = 35

    defaultConfig {
        applicationId = "download.simplevpn"
        minSdk = 24
        targetSdk = 34
        versionCode = 1
        versionName = "0.1.0"
        // The version policy is shared; only this executor changes when a
        // Google Play build is introduced.
        buildConfigField("String", "UPDATE_CHANNEL", "\"direct_apk\"")
    }

    val releaseKeystore = System.getenv("RELEASE_KEYSTORE")
    val releaseStorePassword = System.getenv("RELEASE_STORE_PASSWORD")
    val releaseKeyAlias = System.getenv("RELEASE_KEY_ALIAS")
    val releaseKeyPassword = System.getenv("RELEASE_KEY_PASSWORD")
    val releaseSigningValues = listOf(
        releaseKeystore,
        releaseStorePassword,
        releaseKeyAlias,
        releaseKeyPassword,
    )
    val hasReleaseSigning = releaseSigningValues.all { !it.isNullOrBlank() }

    require(hasReleaseSigning || releaseSigningValues.all { it.isNullOrBlank() }) {
        "Release signing is only accepted when the keystore, both passwords and alias are all present."
    }

    signingConfigs {
        getByName("debug") {
            // The same key on every build, when CI supplies one.
            //
            // Gradle's default is a keystore generated on the spot, which on a
            // fresh build machine means a different key each time. Android
            // then refuses to install the new APK over the old one, a tester
            // uninstalls to get past it, and uninstalling wipes the app data -
            // where the identity of the installation lives. Six devices
            // accumulated on one account that way, and it looked like the
            // identity model was wrong when it was the build.
            //
            // Absent, the local default applies, so a developer machine builds
            // as before.
            System.getenv("DEBUG_KEYSTORE")?.let { path ->
                storeFile = file(path)
                storeType = "PKCS12"
                storePassword = "android"
                keyAlias = "androiddebugkey"
                keyPassword = "android"
            }
        }

        if (hasReleaseSigning) {
            create("release") {
                storeFile = file(releaseKeystore!!)
                storeType = "PKCS12"
                storePassword = releaseStorePassword!!
                keyAlias = releaseKeyAlias!!
                keyPassword = releaseKeyPassword!!
            }
        }
    }

    buildTypes {
        debug {
            isMinifyEnabled = false
            applicationIdSuffix = ".debug"
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            // The release keystore is held by the Business Owner and injected
            // in CI, never stored in the repository. A developer can still
            // assemble an unsigned release locally; publication verifies the
            // APK signature and refuses it. See docs/architecture/secrets-model.md.
            signingConfig = signingConfigs.findByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    sourceSets {
        getByName("main") {
            kotlin.srcDirs("src/main/kotlin")
        }
        getByName("test") {
            kotlin.srcDirs("src/test/kotlin")
        }
    }

    testOptions {
        unitTests {
            // The rules under test are plain Kotlin, but they are compiled
            // against android.jar, whose methods throw by default off a
            // device. Returning defaults instead lets a rule be tested for
            // what it decides rather than for which stub it happened to touch.
            isReturnDefaultValues = true
        }
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }

    // Reproducibility: the APK must not embed the build machine or the moment
    // it was built. Without this, two builds of the same commit differ.
    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.activity.compose)

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)

    // The engine and the packet bridge arrive as one locally built AAR, not
    // from a third-party Maven republish. See ADR-020 in
    // docs/architecture/decisions.md. They share one artifact because each
    // gomobile binding carries its own Go runtime and two of them in one APK
    // fail the build on duplicated runtime classes.
    // The directory is empty until the Android Build workflow produces it.
    implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.aar"))))

    testImplementation(libs.junit)
    testImplementation(libs.json)
}
