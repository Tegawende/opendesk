// How the People and Companies lists can be ordered, first the default (contracts/api.md "Contacts").
import type { CompanySort, ContactSort } from "./api/endpoints";

export const PEOPLE_SORTS: ContactSort[] = ["-created", "created", "name", "-name", "-tickets", "tickets"];
export const COMPANY_SORTS: CompanySort[] = ["name", "-name", "created", "-created", "contacts", "-contacts"];
