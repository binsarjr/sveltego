import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  const config = vscode.workspace.getConfiguration("sveltego");
  const binary = config.get<string>("lsp.path", "sveltego-lsp");

  const serverOptions: ServerOptions = {
    run: { command: binary, transport: TransportKind.stdio },
    debug: { command: binary, transport: TransportKind.stdio },
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "svelte" }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/*.{svelte,go}"),
    },
    outputChannelName: "sveltego",
  };

  client = new LanguageClient(
    "sveltego",
    "sveltego",
    serverOptions,
    clientOptions
  );

  try {
    await client.start();
  } catch (err) {
    void vscode.window.showErrorMessage(
      `sveltego: failed to start LSP at ${binary}. ${
        err instanceof Error ? err.message : String(err)
      }`
    );
  }

  context.subscriptions.push({
    dispose: () => {
      void client?.stop();
    },
  });
}

export async function deactivate(): Promise<void> {
  await client?.stop();
}
