import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useBranding } from '../contexts/BrandingContext';

// Generates a full shade palette from a single hex color.
// Uses HSL lightness shifting for a consistent palette.
function generatePalette(hex: string): Record<string, string> {
  const r = parseInt(hex.slice(1, 3), 16) / 255;
  const g = parseInt(hex.slice(3, 5), 16) / 255;
  const b = parseInt(hex.slice(5, 7), 16) / 255;

  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  let h = 0, s = 0;
  const l = (max + min) / 2;

  if (max !== min) {
    const d = max - min;
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
    else if (max === g) h = ((b - r) / d + 2) / 6;
    else h = ((r - g) / d + 4) / 6;
  }

  const hslToHex = (h: number, s: number, l: number): string => {
    const hue2rgb = (p: number, q: number, t: number) => {
      if (t < 0) t += 1; if (t > 1) t -= 1;
      if (t < 1/6) return p + (q - p) * 6 * t;
      if (t < 1/2) return q;
      if (t < 2/3) return p + (q - p) * (2/3 - t) * 6;
      return p;
    };
    let rr, gg, bb;
    if (s === 0) { rr = gg = bb = l; } else {
      const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
      const p = 2 * l - q;
      rr = hue2rgb(p, q, h + 1/3);
      gg = hue2rgb(p, q, h);
      bb = hue2rgb(p, q, h - 1/3);
    }
    const toHex = (v: number) => Math.round(v * 255).toString(16).padStart(2, '0');
    return `#${toHex(rr)}${toHex(gg)}${toHex(bb)}`;
  };

  // Map shade levels to target lightness values
  const shades: Record<string, number> = {
    '50': 0.95, '100': 0.90, '200': 0.80, '300': 0.70,
    '400': 0.55, '500': 0.45, '600': 0.38, '700': 0.30,
    '800': 0.22, '900': 0.15,
  };

  const result: Record<string, string> = {};
  for (const [shade, targetL] of Object.entries(shades)) {
    result[shade] = hslToHex(h, s, targetL);
  }
  return result;
}

const isValidHex = (c: string) => /^#[0-9a-fA-F]{6}$/.test(c);

export default function BrandingThemeInjector() {
  const { branding, loaded } = useBranding();
  const location = useLocation();
  const isAdmin = location.pathname.startsWith('/last');

  // Apply CSS custom properties for theme colors
  useEffect(() => {
    if (!loaded) return;
    const root = document.documentElement;

    if (branding.primaryColor && isValidHex(branding.primaryColor)) {
      const palette = generatePalette(branding.primaryColor);
      for (const [shade, color] of Object.entries(palette)) {
        root.style.setProperty(`--color-primary-${shade}`, color);
      }
    }

    if (branding.fontFamily) {
      root.style.setProperty('--font-sans', `'${branding.fontFamily}', system-ui, -apple-system, sans-serif`);
    }
    if (branding.headingFont) {
      root.style.setProperty('--font-heading', `'${branding.headingFont}', system-ui, -apple-system, sans-serif`);
    }

    return () => {
      // Cleanup: remove custom properties on unmount
      for (let i = 50; i <= 900; i = i < 100 ? 100 : i + 100) {
        root.style.removeProperty(`--color-primary-${i}`);
      }
      root.style.removeProperty('--font-sans');
      root.style.removeProperty('--font-heading');
    };
  }, [loaded, branding.primaryColor, branding.fontFamily, branding.headingFont]);

  // Update page title
  useEffect(() => {
    if (loaded && branding.appName) {
      document.title = branding.appName;
    }
  }, [loaded, branding.appName]);

  // Update favicon
  useEffect(() => {
    if (!loaded) return;
    const link = document.querySelector('link[rel="icon"]') as HTMLLinkElement | null;
    if (branding.faviconUrl && link) {
      link.href = branding.faviconUrl;
    }
  }, [loaded, branding.faviconUrl]);

  // Inject analytics snippet (not on admin pages)
  useEffect(() => {
    if (!loaded || isAdmin) return;
    const snippet = branding.analyticsSnippet;
    if (!snippet || snippet.startsWith('<!--')) return;

    const id = 'branding-analytics';
    let el = document.getElementById(id);
    if (!el) {
      el = document.createElement('div');
      el.id = id;
      el.style.display = 'none';
      document.head.appendChild(el);
    }
    el.innerHTML = snippet;

    return () => {
      const existing = document.getElementById(id);
      if (existing) existing.remove();
    };
  }, [loaded, branding.analyticsSnippet, isAdmin]);

  // Inject custom CSS (not on admin pages)
  useEffect(() => {
    if (!loaded || isAdmin) return;
    const css = branding.customCss;
    if (!css) return;

    const id = 'branding-custom-css';
    let style = document.getElementById(id) as HTMLStyleElement | null;
    if (!style) {
      style = document.createElement('style');
      style.id = id;
      document.head.appendChild(style);
    }
    style.textContent = css;

    return () => {
      const existing = document.getElementById(id);
      if (existing) existing.remove();
    };
  }, [loaded, branding.customCss, isAdmin]);

  // Inject custom head HTML (not on admin pages)
  useEffect(() => {
    if (!loaded || isAdmin) return;
    const html = branding.headHtml;
    if (!html) return;

    const id = 'branding-head-html';
    let el = document.getElementById(id);
    if (!el) {
      el = document.createElement('div');
      el.id = id;
      el.style.display = 'none';
      document.head.appendChild(el);
    }
    el.innerHTML = html;

    return () => {
      const existing = document.getElementById(id);
      if (existing) existing.remove();
    };
  }, [loaded, branding.headHtml, isAdmin]);

  return null;
}
