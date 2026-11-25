import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

export type Model<K extends keyof components["schemas"]> =
  components["schemas"][K];
export const api = createClient<paths>({
  credentials: "same-origin",
  cache: "no-store",
});
export function headers(token: string) {
  return { "X-CSRF-Token": token, Origin: window.location.origin };
}
export function value<T>(result: { data?: T; error?: Model<"Error"> }): T {
  if (result.error) throw new Error(result.error.error.message);
  if (result.data === undefined)
    throw new Error("The service returned an empty response.");
  return result.data;
}
export function statusText(status: string) {
  return status
    .toLowerCase()
    .replaceAll("_", " ")
    .replace(/^./, (c) => c.toUpperCase());
}
