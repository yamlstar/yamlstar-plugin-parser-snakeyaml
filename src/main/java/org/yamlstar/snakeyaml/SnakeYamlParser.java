package org.yamlstar.snakeyaml;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import org.snakeyaml.engine.v2.api.LoadSettings;
import org.snakeyaml.engine.v2.api.lowlevel.Parse;

/** Thread-safe parser shared by JVM consumers and the native plugin. */
public final class SnakeYamlParser {
    public static final String VERSION = "0.1.0";

    private SnakeYamlParser() { }

    /**
     * Parse unchanged input, including stream boundaries and zero-based marks.
     *
     * @param input YAML text, never rewritten or newline-normalized
     * @return immutable events with immutable metadata
     * @throws NullPointerException if input is null
     * @throws org.snakeyaml.engine.v2.exceptions.YamlEngineException on bad YAML
     */
    public static List<ParseEvent> parse(String input) {
        Objects.requireNonNull(input, "input");
        Parse parser = new Parse(LoadSettings.builder().setUseMarks(true).build());
        List<ParseEvent> events = new ArrayList<>();
        parser.parseString(input).forEach(event -> events.add(new ParseEvent(event)));
        return List.copyOf(events);
    }
}
