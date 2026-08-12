interface PlausibleOptions {
  props?: Record<string, string>;
  callback?: () => void;
}

interface Window {
  plausible?: (event: string, options?: PlausibleOptions) => void;
}
