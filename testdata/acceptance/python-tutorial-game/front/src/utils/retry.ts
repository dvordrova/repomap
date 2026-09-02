export function retryImport(path: string, retriesLeft = 5, interval = 1000) {
  return new Promise((resolve, reject) => {
    import(path).then(resolve).catch((error: any) => {
      setTimeout(() => {
        if (retriesLeft === 1) {
          // reject('maximum retries exceeded');
          reject(error);
          return;
        }

        // Passing on "reject" is the important part
        retryImport(path, retriesLeft - 1, interval).then(resolve, reject);
      }, interval);
    });
  });
}
