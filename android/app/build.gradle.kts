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
            // Signing is intentionally absent. The release keystore is held by
            // the Business Owner and injected in CI, never stored in the repo.
            // See docs/architecture/secrets-model.md.
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    // A format string with a placeholder the formatter cannot read throws at
    // the moment the text is drawn, not when the build runs - so `%1` instead
    // of `%1$s` compiled, shipped, and closed the application on the first tap
    // of the button that used it.
    //
    // Restricted to exactly these checks on purpose. A blanket lint run would
    // fail on unrelated style opinions and get switched off within a week; this
    // one catches a class of bug that reaches people and nothing else.
    lint {
        abortOnError = true
        checkOnly += listOf("StringFormatInvalid", "StringFormatMatches", "StringFormatCount")
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
}
