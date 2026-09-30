package pluginversion
import "testing"
func TestStrictReleaseVersions(t *testing.T) {
 for _,value:=range []string{"1.0","v1.0.0","01.0.0","+1.0.0","1.0.0-beta","1.0.0+build","1.0.-1"} { if _,ok:=Parse(value); ok { t.Errorf("accepted %q",value) } }
 if !Newer("1.10.0","1.9.0") || Newer("1.9.0","1.10.0") || Newer("1.0.0","1.0.0") { t.Fatal("incorrect version order") }
}
