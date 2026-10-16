import en from "./locales/en.json";
import zh from "./locales/zh.json";

export type CatalogMessages = typeof en;
export const catalogMessages: Record<string, CatalogMessages> = { en, zh };
