plugins { id("com.android.application") }

android {
    namespace = "io.github.ftomza.lanos"
    compileSdk = 37
    defaultConfig {
        applicationId = "io.github.ftomza.lanos"
        minSdk = 26
        targetSdk = 35
        versionCode = 14
        versionName = "0.9.1-live"
        manifestPlaceholders["lanosLabel"] = "Lanos"
    }
    buildTypes.getByName("debug") {
        applicationIdSuffix = ".debug"
        manifestPlaceholders["lanosLabel"] = "Lanos LIVE"
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    lint { abortOnError = true }
}

kotlin { compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) } }

abstract class DashboardAssets : DefaultTask() {
    @get:InputDirectory
    @get:PathSensitive(PathSensitivity.RELATIVE)
    abstract val webRoot: DirectoryProperty

    @get:OutputDirectory
    abstract val outputDirectory: DirectoryProperty

    @get:javax.inject.Inject
    abstract val files: FileSystemOperations

    @TaskAction
    fun copyDashboard() {
        files.sync {
            from(webRoot) {
                include("index.html", "style.css", "app.mjs", "model.mjs", "fuel.mjs", "fuel-controls.mjs", "analytics-model.mjs", "history-chart.mjs")
                filesMatching("index.html") {
                    filter { line: String ->
                        line.replace("</head>", """
                              <link rel="stylesheet" href="/android.css">
                              <link rel="stylesheet" href="/dtc.css">
                            </head>
                            """.trimIndent())
                            .replace("<div class=\"header-status\">",
                                "<div class=\"header-status\">" +
                                    "<a class=\"probe-link adapter-check-link\" href=\"/vgate-check\">Проверить Vgate</a>" +
                                    "<a class=\"probe-link\" href=\"/live\">Подключить авто</a>" +
                                    "<a class=\"probe-link simulator-link\" href=\"/simulator\">Симулятор</a>")
                            .replace("<span id=\"connection\"",
                                "<button id=\"mil-status\" type=\"button\" class=\"mil-badge\" aria-haspopup=\"dialog\" aria-controls=\"mil-details\" hidden></button><span id=\"connection\"")
                            .replace("<button id=\"reconnect\"",
                                "<a class=\"probe-link vgate-power-link\" href=\"/vgate-power-cycle\">Питание Vgate отключал</a><button id=\"reconnect\"")
                            .replace("<div class=\"fuel-actions\">",
                                "<details class=\"fuel-options\"><summary>Настройки</summary><div class=\"fuel-actions\">")
                            .replace("Калибровка</button></div></div>",
                                "Калибровка</button><a class=\"probe-link fuel-iat-link\" href=\"/fuel-iat\">Оценка LIVE по IAT</a><a class=\"probe-link\" href=\"/trip-journal\">Журнал LIVE</a></div></details></div>")
                            .replace("</body>", """
                              <dialog id="mil-details" aria-labelledby="mil-details-title">
                                <h2 id="mil-details-title">Check Engine</h2>
                                <div id="mil-dtc-results"></div>
                                <div class="dialog-actions"><button type="button" data-close autofocus>Закрыть</button></div>
                              </dialog>
                            </body>
                            """.trimIndent())
                    }
                }
                filesMatching("app.mjs") {
                    filter { line: String ->
                        line.replace("import { Feed } from './feed.mjs';",
                            "import { Feed } from './feed.mjs';\nimport { bindMilDetails } from './mil.mjs';")
                            .replace("const bars = ", "const renderMil = bindMilDetails(elements['mil-status'], elements['mil-details']);\nconst bars = ")
                            .replace("  elements.connection.textContent = text.connection;",
                                "  elements.connection.textContent = text.connection;\n" +
                                    "  renderMil(event, performance.now() - eventAt, transport.status);")
                    }
                }
                filesMatching("model.mjs") {
                    filter { line: String ->
                        line.replace("Автоматические попытки завершены. Проверьте сервер и повторите подключение.",
                            "Приборка не отвечает. Перезапустите экран.")
                    }
                }
            }
            into(outputDirectory)
        }
    }
}

androidComponents.onVariants { variant ->
    val assetsTask = tasks.register<DashboardAssets>("copyDashboardAssetsFor${variant.name.replaceFirstChar { it.uppercaseChar() }}") {
        webRoot.set(rootProject.layout.projectDirectory.dir("../web"))
        outputDirectory.set(layout.buildDirectory.dir("generated/dashboardAssets/${variant.name}"))
    }
    variant.sources.assets?.addGeneratedSourceDirectory(assetsTask, DashboardAssets::outputDirectory)
}

dependencies {
    testImplementation("junit:junit:4.13.2")
    // Android provides org.json at runtime; JVM tests use its real implementation.
    testImplementation("org.json:json:20250517")
}

tasks.withType<Test>().configureEach {
    val fixture = rootProject.layout.projectDirectory.file("../internal/elm327/testdata/vgate-ble-2026-10-24.json")
    inputs.file(fixture).withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("lanos.ble.fixture", fixture.asFile.absolutePath)
}

tasks.withType<Test>().configureEach {
    val fixture = rootProject.layout.projectDirectory.file("../internal/fuel/testdata/android-golden.json")
    inputs.file(fixture).withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("lanos.fuel.fixture", fixture.asFile.absolutePath)
}

tasks.withType<Test>().configureEach {
    val fixture = rootProject.layout.projectDirectory.file("../internal/elm327/testdata/vgate-pending-2026-10-29.json")
    inputs.file(fixture).withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("lanos.mil.fixture", fixture.asFile.absolutePath)
}
