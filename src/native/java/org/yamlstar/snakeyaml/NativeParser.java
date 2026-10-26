package org.yamlstar.snakeyaml;

import java.nio.ByteBuffer;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import org.graalvm.nativeimage.IsolateThread;
import org.graalvm.nativeimage.c.function.CEntryPoint;
import org.graalvm.nativeimage.c.function.CFunction;
import org.graalvm.nativeimage.c.type.CCharPointer;
import org.graalvm.nativeimage.c.type.CCharPointerPointer;
import org.graalvm.word.UnsignedWord;
import org.graalvm.word.Pointer;
import org.graalvm.word.WordFactory;

/** Private isolate-aware entry point wrapped by the public C shim. */
public final class NativeParser {
    private NativeParser() { }

    @CFunction("malloc")
    private static native CCharPointer allocate(UnsignedWord size);

    @CEntryPoint(name = "snakeyaml_parse")
    public static int parse(IsolateThread thread, CCharPointer input,
            UnsignedWord length, CCharPointer options, UnsignedWord optionsLength,
            CCharPointerPointer output, Pointer outputLength) {
        try {
            if (length.rawValue() < 0 || length.rawValue() > Integer.MAX_VALUE
                    || optionsLength.rawValue() < 0
                    || optionsLength.rawValue() > Integer.MAX_VALUE) {
                return reply("Input exceeds Java string size", 1, output, outputLength);
            }
            // Hosts pass an empty map. Reject options the parser cannot implement.
            String config = decode(options, (int) optionsLength.rawValue()).trim();
            if (!config.isEmpty() && !config.equals("{}") && !config.equals("nil")) {
                return reply("SnakeYAML parser accepts no options", 1, output, outputLength);
            }
            String yaml = decode(input, (int) length.rawValue());
            return reply(EventEdn.write(SnakeYamlParser.parse(yaml)),
                0, output, outputLength);
        } catch (Throwable error) {
            try {
                return reply(error.toString(), 1, output, outputLength);
            } catch (Throwable ignored) {
                return 2;
            }
        }
    }

    private static String decode(CCharPointer source, int length)
            throws java.nio.charset.CharacterCodingException {
        byte[] bytes = new byte[length];
        for (int i = 0; i < length; i++) bytes[i] = source.read(i);
        return StandardCharsets.UTF_8.newDecoder()
            .onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(bytes)).toString();
    }

    private static int reply(String text, int status,
            CCharPointerPointer output, Pointer outputLength) {
        byte[] bytes = text.getBytes(StandardCharsets.UTF_8);
        CCharPointer buffer = allocate(WordFactory.unsigned(Math.max(bytes.length, 1)));
        if (buffer.isNull()) return 2;
        for (int i = 0; i < bytes.length; i++) buffer.write(i, bytes[i]);
        output.write(buffer);
        // size_t is a machine word. Both supported architectures are 64-bit.
        outputLength.writeWord(0, WordFactory.unsigned(bytes.length));
        return status;
    }
}
