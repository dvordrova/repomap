export default class JourneyReporter {
  onTestEnd(test, result) {
    console.log(`${test.title}: ${result.status}`)
  }
}
