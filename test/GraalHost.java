import java.util.List;
import org.graalvm.nativeimage.c.CContext;
import org.graalvm.nativeimage.c.function.CFunction;
import org.graalvm.nativeimage.c.type.CCharPointer;
import org.graalvm.nativeimage.c.type.CTypeConversion;

/** Exercise a plugin-owned isolate from another Native Image executable. */
@CContext(GraalHost.Headers.class)
public class GraalHost {
    @CFunction("snakeyaml_probe")
    private static native int probe(CCharPointer path);

    public static void main(String[] args) {
        try (var path = CTypeConversion.toCString(args[0])) {
            System.exit(probe(path.get()));
        }
    }

    public static class Headers implements CContext.Directives {
        public List<String> getHeaderFiles() {
            return List.of("\"graal-host.h\"");
        }
    }
}
