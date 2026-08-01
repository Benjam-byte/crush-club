import {
  d
} from "./chunk-5C4KPRKN.js";

// ../../node_modules/.pnpm/@ionic+core@8.8.16/node_modules/@ionic/core/components/p-CIGNaXM1.js
var r = () => {
  if (void 0 !== d) return d.Capacitor;
};

// ../../node_modules/.pnpm/@ionic+core@8.8.16/node_modules/@ionic/core/components/p-D13Eaw-8.js
var n;
var i;
!(function(o) {
  o.Unimplemented = "UNIMPLEMENTED", o.Unavailable = "UNAVAILABLE";
})(n || (n = {})), (function(o) {
  o.Body = "body", o.Ionic = "ionic", o.Native = "native", o.None = "none";
})(i || (i = {}));
var t = { getEngine() {
  const n2 = r();
  if (null == n2 ? void 0 : n2.isPluginAvailable("Keyboard")) return n2.Plugins.Keyboard;
}, getResizeMode() {
  const o = this.getEngine();
  return (null == o ? void 0 : o.getResizeMode) ? o.getResizeMode().catch(((o2) => {
    if (o2.code !== n.Unimplemented) throw o2;
  })) : Promise.resolve(void 0);
} };

export {
  r,
  i,
  t
};
//# sourceMappingURL=chunk-Y2ZZXDS7.js.map
