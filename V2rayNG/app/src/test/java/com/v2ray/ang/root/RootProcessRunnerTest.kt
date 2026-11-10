package com.v2ray.ang.root

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import java.io.File
import java.io.FileOutputStream
import java.util.jar.Attributes
import java.util.jar.JarOutputStream
import java.util.jar.Manifest
import java.util.concurrent.TimeUnit
import kotlin.system.exitProcess

class RootProcessRunnerTest {
    @Test fun `a silent blocked command respects its deadline`() {
        val start = System.nanoTime()
        val result = RootProcessRunner.run(testChild("sleep", "5000"), 150)
        assertEquals(-1, result.code)
        assertTrue(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start) < 1500)
    }
    @Test fun `inherited stdout cannot hold a completed command open`() {
        val start = System.nanoTime()
        val result = RootProcessRunner.run(testChild("detach", "5000"), 3000)
        assertEquals(0, result.code)
        assertTrue(result.output.contains("complete"))
        assertTrue(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start) < 1500)
    }
    @Test fun `large output is drained without unbounded retention`() {
        val result = RootProcessRunner.run(testChild("output", "1048576"), 3000, 4096)
        assertEquals(0, result.code)
        assertEquals(4096, result.output.length)
    }
    @Test fun `failure status and diagnostic output survive`() {
        val result = RootProcessRunner.run(testChild("fail"), 1000)
        assertEquals(4, result.code)
        assertTrue(result.output.contains("lock-unavailable"))
    }

    class TestChild {
        companion object {
            @JvmStatic
            fun main(arguments: Array<String>) {
                when (arguments[0]) {
                    "sleep" -> Thread.sleep(arguments[1].toLong())
                    "detach" -> {
                        ProcessBuilder(RootProcessRunnerTest.testChild("sleep", arguments[1])).inheritIO().start()
                        println("complete")
                    }
                    "output" -> repeat(arguments[1].toInt() / 2) { print("x\n") }
                    "fail" -> {
                        System.err.println("lock-unavailable")
                        exitProcess(4)
                    }
                }
            }
        }
    }

    companion object {
        private fun testChild(vararg arguments: String): List<String> {
            val javaHome = System.getProperty("java.home")
            val java = File(javaHome, "bin/java.exe").takeIf(File::isFile)
                ?: File(javaHome, "bin/java")
            return listOf(java.absolutePath, "-cp", runtimeClasspathJar().absolutePath, TestChild::class.java.name) + arguments
        }

        private fun runtimeClasspathJar(): File {
            val manifest = Manifest().apply {
                mainAttributes[Attributes.Name.MANIFEST_VERSION] = "1.0"
                mainAttributes[Attributes.Name.CLASS_PATH] = System.getProperty("java.class.path").orEmpty()
                    .split(File.pathSeparator)
                    .joinToString(" ") { File(it).toURI().toURL().toString() }
            }
            return File.createTempFile("root-runner-test-", ".jar").apply {
                deleteOnExit()
                JarOutputStream(FileOutputStream(this), manifest).close()
            }
        }
    }
}
