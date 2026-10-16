import en from "../../resources/locales/en.json";
import zh from "../../resources/locales/zh.json";
export { en, zh };
export type Messages = typeof en;
export const catalogs: Record<string, Messages> = { en, zh };
