package org.yamlstar.snakeyaml;

import java.util.List;
import java.util.Map;

/** EDN event serialization used by the ABI 2 event-source interface. */
public final class EventEdn {
    private EventEdn() { }

    public static String write(List<ParseEvent> events) {
        StringBuilder out = new StringBuilder("[");
        for (ParseEvent event : events) {
            out.append("{:event ");
            quote(out, event.getEvent());
            field(out, "value", event.getValue());
            field(out, "style", event.getStyle().isEmpty() ? null : event.getStyle());
            field(out, "anchor", event.getAnchor());
            field(out, "tag", event.getTag());
            field(out, "name", event.getName());
            field(out, "version", event.getVersion());
            if (event.getEvent().equals("mapping_start")
                    || event.getEvent().equals("sequence_start")) {
                out.append(" :flow ").append(event.isFlow());
            }
            if (event.isExplicit()) out.append(" :explicit true");
            if (!event.getTags().isEmpty()) {
                out.append(" :tags {");
                for (Map.Entry<String, String> tag : event.getTags().entrySet()) {
                    quote(out, tag.getKey());
                    out.append(' ');
                    quote(out, tag.getValue());
                    out.append(' ');
                }
                out.append('}');
            }
            mark(out, "start-mark", event.getStartMark());
            mark(out, "end-mark", event.getEndMark());
            out.append("} ");
        }
        return out.append(']').toString();
    }

    private static void mark(StringBuilder out, String key, SourceMark mark) {
        if (mark == null) return;
        out.append(" :").append(key).append(" {:line ").append(mark.getLine())
            .append(" :column ").append(mark.getColumn())
            .append(" :index ").append(mark.getIndex()).append('}');
    }

    private static void field(StringBuilder out, String key, String value) {
        if (value == null) return;
        out.append(" :").append(key).append(' ');
        quote(out, value);
    }

    private static void quote(StringBuilder out, String value) {
        out.append('"');
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            switch (c) {
                case '"': out.append("\\\""); break;
                case '\\': out.append("\\\\"); break;
                case '\n': out.append("\\n"); break;
                case '\r': out.append("\\r"); break;
                case '\t': out.append("\\t"); break;
                case '\b': out.append("\\b"); break;
                case '\f': out.append("\\f"); break;
                default:
                    if (c < 32 || c == 127) {
                        out.append(String.format("\\u%04x", (int) c));
                    } else out.append(c);
            }
        }
        out.append('"');
    }

    /** Print a normalized event vector for parity checks. */
    public static void main(String[] args) throws java.io.IOException {
        String input = new String(System.in.readAllBytes(),
            java.nio.charset.StandardCharsets.UTF_8);
        System.out.print(write(SnakeYamlParser.parse(input)));
    }
}
