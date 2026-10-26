package org.yamlstar.snakeyaml;

import java.util.Objects;
import org.snakeyaml.engine.v2.exceptions.Mark;

/** Immutable zero-based position. Indices and columns count Unicode code points. */
public final class SourceMark {
    private final int line;
    private final int column;
    private final int index;

    SourceMark(Mark mark) {
        line = mark.getLine();
        column = mark.getColumn();
        index = mark.getIndex();
    }

    public int getLine() { return line; }
    public int getColumn() { return column; }
    public int getIndex() { return index; }

    @Override
    public boolean equals(Object other) {
        if (!(other instanceof SourceMark)) return false;
        SourceMark mark = (SourceMark) other;
        return line == mark.line && column == mark.column && index == mark.index;
    }

    @Override
    public int hashCode() { return Objects.hash(line, column, index); }
}
