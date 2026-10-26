package org.yamlstar.snakeyaml;

import static org.junit.jupiter.api.Assertions.*;
import java.util.List;
import java.util.ArrayList;
import java.util.concurrent.Callable;
import java.util.concurrent.Executors;
import org.junit.jupiter.api.Test;

class SnakeYamlParserTest {
    @Test
    void versionMetadata() throws java.io.IOException {
        String manifest = java.nio.file.Files.readString(java.nio.file.Path.of("plugin.edn"));
        assertTrue(manifest.contains(":version \"" + SnakeYamlParser.VERSION + "\""));
    }

    @Test
    void boundaries() {
        assertEquals("stream_start", SnakeYamlParser.parse("").get(0).getEvent());
        assertEquals(2, SnakeYamlParser.parse("").size());
        List<ParseEvent> events = SnakeYamlParser.parse("--- value\n...\n");
        assertEquals(5, events.size());
        assertTrue(events.get(1).isExplicit());
        assertTrue(events.get(3).isExplicit());
        assertEquals("stream_end", events.get(4).getEvent());
    }

    @Test
    void directivesAnchorsTagsAndAliases() {
        List<ParseEvent> events = SnakeYamlParser.parse(
            "%YAML 1.2\n%TAG !e! tag:example.com,2026:\n" +
            "--- &a !e!items [!!str 1, 'two', *a]\n...\n");
        assertEquals("1.2", events.get(1).getVersion());
        assertEquals("tag:example.com,2026:", events.get(1).getTags().get("!e!"));
        assertEquals("a", events.get(2).getAnchor());
        assertTrue(events.get(2).isFlow());
        assertEquals("tag:example.com,2026:items", events.get(2).getTag());
        assertEquals("tag:yaml.org,2002:str", events.get(3).getTag());
        assertEquals("single", events.get(4).getStyle());
        assertEquals("a", events.get(5).getName());
        assertEquals("alias", events.get(5).getEvent());
    }

    @Test
    void styles() {
        List<ParseEvent> events = SnakeYamlParser.parse(
            "[plain, 'single', \"double\"]\n");
        assertEquals("", events.get(3).getStyle());
        assertEquals("single", events.get(4).getStyle());
        assertEquals("double", events.get(5).getStyle());
        assertEquals("literal", SnakeYamlParser.parse("|\n  line").get(2).getStyle());
        assertEquals("folded", SnakeYamlParser.parse(">\n  line").get(2).getStyle());
    }

    @Test
    void originalPositionsAndUnicode() {
        List<ParseEvent> events = SnakeYamlParser.parse("[🙂, λ]\r\n");
        assertEquals(0, events.get(3).getStartMark().getLine());
        assertEquals(1, events.get(3).getStartMark().getColumn());
        assertEquals(1, events.get(3).getStartMark().getIndex());
        assertEquals(4, events.get(4).getStartMark().getColumn());
        assertEquals(4, events.get(4).getStartMark().getIndex());
        ParseEvent scalar = SnakeYamlParser.parse("hello").get(2);
        assertEquals(5, scalar.getEndMark().getIndex());
        assertEquals(5, scalar.getEndMark().getColumn());
        assertEquals(0, scalar.getEndMark().getLine());
    }

    @Test
    void noInsertedFinalNewline() {
        List<ParseEvent> events = SnakeYamlParser.parse("|\n  line");
        assertEquals("line\n", events.get(2).getValue());
        assertEquals(8, events.get(2).getEndMark().getIndex());
    }

    @Test
    void immutableCollections() {
        List<ParseEvent> events = SnakeYamlParser.parse(
            "%TAG !e! tag:example.com,2026:\n--- !e!x value");
        assertThrows(UnsupportedOperationException.class, events::clear);
        assertThrows(UnsupportedOperationException.class,
            () -> events.get(1).getTags().put("!x!", "bad"));
    }

    @Test
    void malformedInput() {
        for (String yaml : List.of("[unterminated", "*missing: [", "\u0000")) {
            assertThrows(RuntimeException.class, () -> SnakeYamlParser.parse(yaml));
        }
        assertThrows(NullPointerException.class, () -> SnakeYamlParser.parse(null));
    }

    @Test
    void concurrentCalls() throws Exception {
        var pool = Executors.newFixedThreadPool(8);
        try {
            List<Callable<String>> calls = new ArrayList<>();
            for (int i = 0; i < 100; i++) {
                calls.add(() -> SnakeYamlParser.parse("[🙂, value]")
                    .get(3).getValue());
            }
            for (var result : pool.invokeAll(calls)) assertEquals("🙂", result.get());
        } finally {
            pool.shutdownNow();
        }
    }

    @Test
    void ednEscapingAndMetadata() {
        String edn = EventEdn.write(SnakeYamlParser.parse(
            "[\"a\\0b\\n\\t\\\"\\\\\", '🙂']"));
        assertTrue(edn.contains("a\\u0000b\\n\\t\\\"\\\\"));
        assertTrue(edn.contains("🙂"));
        assertTrue(edn.contains(":start-mark {:line 0 :column 0 :index 0}"));
    }
}
