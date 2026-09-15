// Cross-context message contracts shared by the page and service worker builds.

interface ConfigureUserMessage {
  type: "configure-user";
  userID: string;
}
