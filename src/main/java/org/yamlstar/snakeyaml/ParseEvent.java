package org.yamlstar.snakeyaml;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import org.snakeyaml.engine.v2.events.*;

/** Immutable normalized event, independent of any host's event representation. */
public final class ParseEvent {
    private final String event;
    private final String value;
    private final String style;
    private final String anchor;
    private final String tag;
    private final String name;
    private final String version;
    private final Map<String, String> tags;
    private final boolean flow;
    private final boolean explicit;
    private final SourceMark startMark;
    private final SourceMark endMark;

    ParseEvent(Event source) {
        switch (source.getEventId()) {
            case StreamStart: event = "stream_start"; break;
            case StreamEnd: event = "stream_end"; break;
            case DocumentStart: event = "document_start"; break;
            case DocumentEnd: event = "document_end"; break;
            case MappingStart: event = "mapping_start"; break;
            case MappingEnd: event = "mapping_end"; break;
            case SequenceStart: event = "sequence_start"; break;
            case SequenceEnd: event = "sequence_end"; break;
            case Scalar: event = "scalar"; break;
            case Alias: event = "alias"; break;
            default: throw new IllegalArgumentException("Unexpected event: " + source);
        }
        startMark = source.getStartMark().map(SourceMark::new).orElse(null);
        endMark = source.getEndMark().map(SourceMark::new).orElse(null);
        anchor = source instanceof NodeEvent && !(source instanceof AliasEvent)
            ? ((NodeEvent) source).getAnchor().map(Object::toString).orElse(null)
            : null;
        name = source instanceof AliasEvent
            ? ((AliasEvent) source).getAlias().toString() : null;
        flow = source instanceof CollectionStartEvent
            && ((CollectionStartEvent) source).isFlow();
        if (source instanceof ScalarEvent) {
            ScalarEvent scalar = (ScalarEvent) source;
            value = scalar.getValue();
            tag = scalar.getTag().orElse(null);
            switch (scalar.getScalarStyle()) {
                case SINGLE_QUOTED: style = "single"; break;
                case DOUBLE_QUOTED: style = "double"; break;
                case LITERAL: style = "literal"; break;
                case FOLDED: style = "folded"; break;
                default: style = "";
            }
        } else {
            value = null;
            style = "";
            tag = source instanceof CollectionStartEvent
                ? ((CollectionStartEvent) source).getTag().orElse(null) : null;
        }
        if (source instanceof DocumentStartEvent) {
            DocumentStartEvent doc = (DocumentStartEvent) source;
            explicit = doc.isExplicit();
            version = doc.getSpecVersion()
                .map(v -> v.getMajor() + "." + v.getMinor()).orElse(null);
            tags = Collections.unmodifiableMap(new LinkedHashMap<>(doc.getTags()));
        } else {
            explicit = source instanceof DocumentEndEvent
                && ((DocumentEndEvent) source).isExplicit();
            version = null;
            tags = Collections.emptyMap();
        }
    }

    public String getEvent() { return event; }
    public String getValue() { return value; }
    public String getStyle() { return style; }
    public String getAnchor() { return anchor; }
    public String getTag() { return tag; }
    public String getName() { return name; }
    public String getVersion() { return version; }
    public Map<String, String> getTags() { return tags; }
    public boolean isFlow() { return flow; }
    public boolean isExplicit() { return explicit; }
    public SourceMark getStartMark() { return startMark; }
    public SourceMark getEndMark() { return endMark; }
}
